# GPT Auto / Shadow Routing (Phase 1)

> 中文摘要：GPT Auto 第一阶段只做**影子观测**。抽样请求的脱敏摘录在后台交给 Jev（TypeSafe System One）分类，得到推荐模型与推理强度，经策略引擎校验后写入决策日志并在管理后台展示。客户请求的模型、推理强度、账号调度、SSE 响应与计费**完全不变**；不会调用推荐的 GPT 模型，`gpt-auto` 也不对客户开放。功能默认关闭（`shadow_enabled=false`、`sample_rate=0`）。入口：管理后台 → 智能运维 → **GPT Auto 影子路由**（`/admin/gpt-auto`）。

## What it does

For a sampled, in-scope request the gateway:

1. Builds a bounded, redacted snapshot of the request (latest user message plus limited context, endpoint, tool presence, approximate size).
2. Lets the request continue through the existing pipeline untouched.
3. Queues the snapshot on a bounded in-process queue. Background workers send it to Jev in a single System One call with four questions:
   - `target_model` (choice): `gpt-5.6-luna`, `gpt-5.6-sol`, `gpt-6-luna`, `gpt-6.1-sol`
   - `reasoning_effort` (choice): `none` … `max`, asking for the lowest effort likely to succeed
   - `task_complexity` (score, 0–4 rubric)
   - `task_category` (choice, six categories)
4. Runs the deterministic policy engine on Jev's answer (capability matrix, enabled models, tool support, routing weights, margins).
5. Stores the raw Jev answer, the policy decision and hypothetical economics in `gpt_auto_shadow_decisions`.

Jev never receives the customer's requested model or effort, and the model question never mentions price.

### Guarantees (Phase 1)

| Never | How it is enforced |
|---|---|
| Rewrite the model, effort or payload | The hook only reads `body`. Prompt-rule and effort rewrites happen later and produce new buffers. Tested in `TestObserveGPTAutoShadowDoesNotTouchResponseOrBody` and `TestGPTAutoObserveDoesNotMutateRequestBody`. |
| Change account selection or SSE output | The hook runs before slot acquisition and account selection, and it writes nothing to the response. |
| Delay the response | `Observe` does no I/O: one cached config read, an in-memory budget check, a hash, a bounded snapshot and a non-blocking channel send. Jev, Redis and database calls all run on separate workers. |
| Charge customers | Shadow code has no dependency on billing. Jev cost exists only on decision rows. |
| Call a GPT model | The only outbound call is TypeSafe System One. The test lab calls Jev only. |
| Expose `gpt-auto` | No model registration, mapping or route was added. |

## Where the code lives

| Concern | Files |
|---|---|
| System One choice/score decoding | `backend/internal/pkg/typesafe/classify.go` |
| Model pool and capability matrix | `backend/internal/service/gpt_auto_models.go` |
| Settings, defaults and validation | `backend/internal/service/gpt_auto_config.go` (setting key `gpt_auto_shadow_config`) |
| Snapshot, redaction, Jev questions | `backend/internal/service/gpt_auto_classifier.go` |
| Policy engine and economics | `backend/internal/service/gpt_auto_policy.go` |
| Queue, workers, breaker, budgets, admin ops | `backend/internal/service/gpt_auto_shadow_service.go` |
| Storage | `backend/internal/repository/gpt_auto_shadow_repo.go`, `backend/migrations/278_gpt_auto_shadow_decisions.sql` |
| Gateway hook | `backend/internal/handler/gpt_auto_shadow_helper.go`, called from the four OpenAI-compatible handlers |
| Admin API | `backend/internal/handler/admin/gpt_auto_handler.go`, `/api/v1/admin/gpt-auto/*` |
| UI | `frontend/src/views/admin/GPTAutoView.vue`, `frontend/src/components/admin/gptAuto/*` |

Each decision's `request_id` is the id the usage log records for the same request (`client:<id>` or `local:<id>`), so the decision detail can show the real production outcome next to the hypothetical one.

## Observed endpoints

Observed: `POST /v1/chat/completions` and `POST /v1/responses` (and their root and Codex aliases), in both the OpenAI-gateway and Anthropic-gateway handler families.

Excluded, with reasons:

- `/v1/messages`, Gemini and Antigravity native shapes: the candidate pool is OpenAI-family only.
- `/v1/systemone`: that endpoint is the classifier itself, so observing it would recurse.
- `GET /v1/responses` (WebSocket) and realtime: there is no single bounded request body.
- Responses compaction requests (`/responses/compact`, remote compaction v2): they summarise context and carry no user task.
- `count_tokens`, images and videos: there is no reasoning-effort decision to make.

## Capability matrix

Supported efforts come from the gateway's own rules rather than a datasheet:

| Model | Efforts | Source |
|---|---|---|
| gpt-5.6-luna, gpt-5.6-sol, gpt-6-luna | none, low, medium, high, xhigh, max | `supportsOpenAIReasoningEffortMax` keeps `max` for GPT-5.6/6 |
| gpt-6.1-sol | low, medium, high, xhigh, max | `openai.ValidateGPT61SolReasoningEffort` rejects `none` |

On compatible (non-official) upstream accounts the gateway strips `reasoning_effort: "none"`, so `none` there means the upstream default. The matrix is per model; per-channel effort handling is not modelled.

Admins can narrow a model's set but never widen it. When Jev picks an unsupported effort, the policy moves to the nearest supported level above it (or below it if nothing higher exists) and records `effort_unsupported`. For example, `gpt-6.1-sol` + `none` becomes `low`, and both values are stored.

## Policy rules (`shadow-v1`)

1. **Hard constraints** (never overridden): model enabled, endpoint allowed, tools supported if the request has tools.
2. **Model**: keep Jev's choice if eligible. A different eligible model wins only if its Jev probability is within 0.05 of the choice *and* its routing weight is higher. If Jev's choice is ineligible, the eligible model with the highest Jev probability wins, with the same tie-break.
3. **No eligible model**: status `no_eligible_candidate`, and no route is fabricated.
4. **Economics never change the route**: a low margin adds `margin_below_target` but keeps the recommendation.
5. Every decision stores `policy_version` and a content hash of the model profiles and margins (`gpt_auto_policy_snapshots`), so it can be replayed.

The billing multiplier affects only hypothetical revenue. The routing weight affects only near-ties. Neither can make an incompatible model eligible.

## Economics (all hypothetical)

```
revenue        = list_price(input, output) × billing_multiplier
upstream_cost  = cheapest enabled configured channel for the model, else list price
classifier     = Jev tokens × jev-latest price (pricing table; fallback $0.042/M input, $0 output)
gross_margin   = (revenue − upstream_cost − classifier) / revenue      (undefined when revenue = 0)
target         = (per-model min_margin ?? global_min_margin) + profit_safety_buffer
min_multiplier = (upstream_cost + classifier) / ((1 − target) × list_cost)
```

- List prices come from the existing `BillingService.GetModelPricing` chain (LiteLLM, fallback and overrides, stored as USD per token). The UI shows USD per 1M tokens.
- Channel costs are shadow-only overrides per model (several channels allowed). They do not change channel pricing, account rate multipliers or customer charges.
- Input tokens are a character estimate (about 4 characters per token).
- Output tokens use the real 7-day average from `usage_logs` for that model and effort, falling back to the model-wide average. With no history, output is excluded and the decision is labelled `no_output_history`. No fixed per-effort multiplier is assumed.
- Jev confidence values are shown as reported. They are not calibrated probabilities that a model will succeed.
- `cached_input_per_m` is stored but not used yet: the snapshot does not know the cache-hit share.
- Routing weight only breaks a near-tie that Jev's probabilities actually show. If Jev reports no probabilities, Jev's choice stands.

The decision detail separates three kinds of figures:

| Figure | Source |
|---|---|
| **Actual production** (served model, effort, tokens, charged amount, list cost, upstream account cost) | The request's real `usage_logs` row, looked up by request id at display time. Shown as "not found" if the request failed or is still streaming. |
| **Hypothetical alternative** (revenue, upstream cost, margin for the recommended model) | The estimate above. Always labelled hypothetical. |
| **Jev classification cost** | Billed Jev usage for this decision, including retries and 2xx responses with unusable answers. |

## Configuration

All settings are stored as one JSON document under the setting key `gpt_auto_shadow_config`, edited in the admin UI or through `PUT /api/v1/admin/gpt-auto/config`. No environment variables or `config.yaml` changes are required. The backend validates every field and returns `400 GPT_AUTO_CONFIG_INVALID` on bad input. Other nodes pick up changes within 30 seconds; the node that saved them applies them immediately.

| Field | Default | Notes |
|---|---|---|
| `shadow_enabled` | `false` | Global switch |
| `sample_rate` | `0` | 0–100 %, deterministic hash of API key + first 4 KiB of body |
| `endpoints` | both | `chat_completions`, `responses` |
| `all_groups` / `group_ids` | `true` / – | Group scope |
| `model_filter` | – | Customer-requested model names; empty means all |
| `classifier_base_url` | `https://api.typesafe.ai` | Must be public HTTPS; resolved IPs are re-checked against DNS rebinding. Rejected if its host matches this gateway's `api_base_url` or `frontend_url`. |
| `classifier_api_key` | – | Write-only. GET returns only a masked hint. Not written to the audit log. Sending the masked hint back keeps the stored key; other values containing `****` are rejected. Changing the classifier host requires re-entering the key. |
| `classifier_proxy_id` | – | Optional existing proxy |
| `classifier_model` | `jev-latest` | |
| `classifier_timeout_ms` | 8000 | 200–60000 per attempt |
| `classifier_max_concurrency` | 4 | Shared with the test lab |
| `classifier_retry_count` | 1 | Retries transport errors, 5xx and 429 only |
| `classifier_daily_call_limit` | 10000 | 0 = unlimited (UTC day, all nodes combined) |
| `classifier_daily_cost_limit` | 5.0 USD | 0 = unlimited (UTC day, all nodes combined) |
| `classification_max_context_chars` | 4000 | Latest user message gets up to 75 % |
| `classification_context_messages` | 4 | Previous turns included |
| `worker_count` / `queue_size` | 2 / 4096 | Per node. Bounded by count and by a 64 MiB snapshot memory cap; overflow is dropped and counted |
| `decision_retention_days` | 30 | Hourly batched cleanup |
| `global_min_margin` / `profit_safety_buffer` | 0.30 / 0.05 | Fractions |
| `model_profiles` | see below | Exactly the four pool models |
| `policy_version` | `shadow-v1` | Stored on each decision |

Initial profiles are the brief's example values, not tuned optima:

| Model | Billing multiplier | Routing weight |
|---|---:|---:|
| gpt-5.6-luna | 1.20 | 60 |
| gpt-5.6-sol | 1.40 | 55 |
| gpt-6-luna | 1.10 | 100 |
| gpt-6.1-sol | 1.50 | 90 |

Each profile also has `description`, which is the capability text Jev sees as that model's choice criterion. The repo has no vendor capability metadata for these models, so the defaults only state generation and tier. **Refine these descriptions before trusting the recommendations.**

## Privacy

When enabled, a redacted, truncated excerpt of sampled requests is sent to the configured Jev provider. The UI shows this notice and asks for confirmation when shadow classification is first enabled. Redaction reuses the content-moderation credential patterns (keys, bearer tokens, JWTs, cookies, long hex/base64 strings, URLs) and also removes emails and phone numbers. Decision rows hold no prompt text. They contain structured features, Jev's answers, the policy decision and user, API key and group IDs for admin filtering. Decisions are visible only through admin-authenticated APIs. Config changes, tests, replays and simulations are recorded in the admin audit log; request bodies are omitted for the config and test routes.

## Reliability

- **Queue**: bounded channel; a full queue drops the sample (`dropped_queue_full`) and never blocks.
- **Workers**: fixed pool started once, live-resized via `worker_count`, panic-safe. Each attempt has its own timeout. Shutdown cancels in-flight Jev calls and still records their outcome.
- **Circuit breaker**: opens after 5 consecutive failures for 30 seconds, then allows one half-open probe.
- **Budgets (cluster-wide)**: every Jev attempt, including retries and test-lab calls, first reserves budget with an atomic Redis Lua script (`gpt_auto:budget:<UTC day>`). The reservation books a pessimistic cost estimate and is then settled to the billed usage, or refunded if no call was sent. Limits therefore do not multiply with the number of gateway nodes. If no shared counter exists for the day (first start or a Redis flush), it is seeded from today's persisted decisions. **If Redis is unreachable while a limit is configured, budgeted classification pauses (fails closed)** and counts `skipped_budget_unavailable`. The request path never waits on Redis or the database: it only reads the last observed cluster usage.
- **Failure isolation**: Jev outages only add `failed` rows. If config loading fails, the last good config is used and the feature stays disabled by default.
- **Policy snapshots** are upserted idempotently; the in-memory "already written" cache expires hourly, so a restored or pruned snapshot table heals itself.

| Scope | What |
|---|---|
| Cluster (Redis) | Daily call and cost budget; per-day counters (each node flushes its deltas about every 10 seconds) |
| Per node | Classifier concurrency (`classifier_max_concurrency`, so the cluster total is nodes × limit), queue and snapshot memory cap, circuit breaker, recent latency ring |
| Database | Decisions, statistics, policy snapshots |

**Overview** shows this node's counters, queue and breaker, the cluster-wide budget for today, and all nodes' counters for today. Logs use the `gpt_auto.*` slog events and never contain prompt text or provider error bodies.

## Admin API

All routes are under `/api/v1/admin/gpt-auto` behind admin auth, the panel rate limit, the audit log and the compliance guard.

| Method | Path | Purpose |
|---|---|---|
| GET | `/meta` | Model pool, native efforts, categories, endpoints, defaults |
| GET / PUT | `/config` | Read (key masked) / validate and save, then re-read |
| GET | `/overview?start_time&end_time&group_id` | Stored stats plus node runtime; max 90-day window |
| GET | `/decisions?page&page_size&start_time&end_time&actual_model&final_model&final_effort&evaluation_status&decision_status&group_id&request_id` | Paginated log |
| GET | `/decisions/:id` | Full Jev output, candidates, rules, economics |
| POST | `/decisions/:id/replay` | `{"mode":"historical"\|"current"\|"custom", ...}`: policy only, no Jev call |
| POST | `/test` | Test lab: calls Jev only, stores nothing |
| POST | `/simulate` | Per-model hypothetical economics for unsaved edits |

## Database migration

`backend/migrations/278_gpt_auto_shadow_decisions.sql` is additive. It creates `gpt_auto_policy_snapshots` and `gpt_auto_shadow_decisions` with indexes on `(created_at, id)`, status, actual model, recommended model, group and request ID. It runs automatically at startup. Nothing existing is altered.

## Where to find the page

`/admin/gpt-auto` (**GPT Auto 影子路由**). The admin sidebar does not render the Smart Ops group: this is pre-existing for every Smart Ops page, because `adminNavGroups` in `AppSidebar.vue` never picks `/admin/smart-ops`. Reach the page in one of these ways:

- The sidebar search (功能搜索, ⌘K): type "GPT Auto".
- The Smart Ops tab bar shown at the top of every Smart Ops page.
- The URL directly.

## Enabling safely in production

1. Deploy with the defaults. Confirm **GPT Auto 影子路由** loads and the overview shows "Disabled" and does not show a "budget store unavailable" warning.
2. In **Models & routing → Jev classifier**, set the API key (and proxy if needed). Keep `jev-latest`.
3. Review the four model descriptions, supported efforts, tool support and any upstream channel costs. Use the simulator to check margins.
4. In **Test lab**, analyse several representative prompts and confirm the results look sensible and the latency is acceptable.
5. Narrow the scope first: turn off `all_groups` and pick one internal group, or set a model filter. Keep the daily call and cost limits low.
6. Enable shadow classification at `sample_rate` 1 %, confirm the privacy notice, and save.
7. Watch **Overview** for error rate, P95 latency, breaker state, drops and spend. Gateway latency should be unchanged.
8. Raise the sample rate and widen the scope gradually. To roll back, switch shadow classification off; it takes effect within 30 seconds on every node.

## Phase 2 notes (not implemented)

- `EvaluateGPTAutoPolicy` is pure and versioned, and it can be called synchronously from a live router.
- Use `replay` (mode `custom`) over stored decisions to tune weights before going live.
- Live routing will need: model registration for `gpt-auto`, a synchronous latency budget with fallback to a default model, model and effort rewriting before channel mapping, billing on the routed model, and a request-level kill switch. None of this exists yet.
