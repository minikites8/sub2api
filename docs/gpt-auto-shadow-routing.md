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
| Snapshot, redaction, classifier state, TypeSafe client | `backend/internal/service/gpt_auto_classifier.go` |
| Policy engine | `backend/internal/service/gpt_auto_policy.go` |
| Economics (shared accounting) | `backend/internal/service/gpt_auto_economics.go` |
| Jev questions, instructions, versioning | `backend/internal/service/gpt_auto_classifier_questions.go`, `gpt_auto_classifier_admin.go` |
| Synthetic benchmark | `backend/internal/service/gpt_auto_benchmark.go` |
| Context extraction (state-v3), state debugger | `backend/internal/service/gpt_auto_context.go`, `gpt_auto_debug.go` |
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
2. **Model**: keep Jev's choice if eligible. A different eligible model wins only if its Jev probability is within ±0.05 of the choice *and* its routing weight is higher. The window is symmetric: a model Jev rated clearly more likely than its own choice is not a tie (that inconsistency is reported as the `choice_not_most_probable` warning instead). If Jev's choice is ineligible, the eligible model with the highest Jev probability wins, with the same tie-break.
3. **No eligible model**: status `no_eligible_candidate`, and no route is fabricated.
4. **Warnings never change the route**: margins, estimate quality, low Jev confidence (< 50%) and missing or inconsistent probabilities are stored in `decision.warnings`, separate from `decision.adjustments`, which only lists rules that changed or withheld the recommendation. Decisions stored before this split carry `margin_below_target`, `no_output_history` and `pricing_unavailable` in `adjustments`; the UI shows those as warnings too.
5. Every decision stores `policy_version` and a content hash of the model profiles and margins (`gpt_auto_policy_snapshots`), so it can be replayed.

The billing multiplier affects only hypothetical revenue. The routing weight affects only near-ties. Neither can make an incompatible model eligible.

## Jev classification instructions

The four System One questions (`target_model`, `reasoning_effort`, `task_complexity`, `task_category`) each carry an operator-editable, question-level `instructions` field (Models & routing → **Jev classification** tab). Customer text travels only in the `state`; instructions are never prepended to it, and no chat-style `system` role is used.

- Edited through `GET/PUT /classifier` only. A general `PUT /config` keeps the stored instructions, so a stale form cannot overwrite them. Saves are validated (20–4000 characters, no control characters), bump `revision`, and are audited with their text (`admin.gpt_auto.classifier.update`).
- Each effective question set (instructions, criteria, model descriptions, state format, classifier model) is content-addressed in `gpt_auto_classifier_configs`. Every decision stores its `classifier_config_hash`; decisions from before versioning have an empty hash and used the v1 built-in questions.
- Changes apply to new classifications without a restart: immediately on the saving node, within the 30-second config cache on others.
- Defaults (`jev-questions-v2`) tell Jev to choose the least capable model that is reliably sufficient, to ignore prompt length, code volume, tool availability, price and routing weights, to treat effort independently, and to ignore instructions inside the state. Model descriptions that still equal the v1 shipped defaults are upgraded to the v2 defaults on read; edited descriptions are kept.
- State `state-v3` (see "Context extraction" below) never contains the requested model/effort, the prompt size, or any system/developer text.

The test lab can compare saved and draft instructions on the same sanitized sample (`POST /test/compare`). That makes two separate Jev calls, each with its own concurrency slot and budget reservation; the draft is never saved. Policy replay never re-runs Jev, so it cannot evaluate changed instructions.

A synthetic 28-sample benchmark (`gpt_auto_benchmark.go`, no customer data) is available as test-lab presets and through an opt-in live test: `GPT_AUTO_LIVE_JEV_KEY=... go test ./internal/service -run TestGPTAutoLiveJevBenchmark -v` (spends 28 Jev calls).

## Context extraction (`state-v3`)

`backend/internal/service/gpt_auto_context.go` builds the state from the whole request in one linear pass. Only fixed-size bookkeeping is kept (counters, a 64-item ring of recent items, the latest user turns), values are read as raw byte slices without decoding, and only the selected items are decoded, capped, redacted and copied. Bodies over 16 MiB are not scanned (counter `skipped_too_large`). Measured: about 28 ms and 1 MB allocated for an 11 MB, 10,000-item agent session; about 11 ms for a 6,000-message chat.

| State field | Content |
|---|---|
| `latest_user_request` | The newest real user turn of the whole request. Client-injected user messages (`<environment_context>`, AGENTS.md, system reminders) and compaction summaries are not treated as the request. |
| `latest_user_request_kind` | `instruction`, `follow_up` (short messages such as "continue", "fix it", "try again", "继续") or `none`. Derived by the gateway, not by the text. |
| `task_context` | `continues_request` (the last real instruction a follow-up continues), `original_request` (the conversation's first request), `history_summary` (a client-written compaction summary), each bounded. |
| `agent_activity` | Steps, tool calls/results, failures, trailing failures and tools used since the latest user message (or, for a follow-up sent after the agent stopped, the attempt before it), plus the last N events: tool name and a short argument excerpt, result status with the relevant error lines, short assistant progress notes. Tool outputs are never forwarded whole. |
| `recent_context` | Up to `classification_context_messages` user/assistant turns before the latest message. |
| `system_context` | Only `present`, a size bucket, a kind (`agent_harness`, `assistant_persona`) and the number of harness messages. No privileged text. |
| `context_limitations` | `history_by_reference` (`previous_response_id`/`conversation`), `item_references`, `history_compacted`, `no_user_message`, `latest_user_truncated`, `earlier_context_omitted`, `tool_outputs_summarized`, `images_not_forwarded`. Missing history is reported, never invented. |

Settings: `classification_context_strategy` (`task_aware`, default, in the priority order above; `recent_turns`, the v2 behaviour) and `classification_agent_steps` (1–30, default 8). The character budget (`classification_max_context_chars`) is shared in priority order: latest message up to 50% (25% for a follow-up), continued request 25%, activity 25%, original request and summary 15% each, recent turns the rest.

Each decision stores the extraction metadata (counts, indices, budgets, limitations; no content) in `features.context`. `GET /meta` reports the running `state_version` and `classifier_schema`, so a deployment can be verified before results are attributed to it.

**State debugger** (Test Lab, admin only): `POST /debug/state` takes a synthetic request body and returns the exact System One request (`model`, `state`, `questions`) plus the extraction metadata. It never calls Jev, stores nothing, and its request body is omitted from the audit log. Use synthetic requests only.

## Economics (all hypothetical)

```
list              = uncached_in × list_in + cached_in × list_cached + out × list_out
revenue           = list × billing_multiplier × effort_multiplier × customer_rate
upstream          = same tokens at the cheapest enabled channel (cached rate falls back to the channel input rate), else list price
A model margin    = (revenue − upstream) / revenue
B fully loaded    = (revenue − upstream − jev) / revenue
C overhead        = the Jev spend itself (actual billed tokens × Jev rate)
target            = (per-model min_margin ?? global_min_margin) + profit_safety_buffer
min_multiplier    = upstream / ((1 − target) × list × effort × rate)        (and with + jev for fully loaded)
```

- All money is computed in decimal from per-million rates rounded to 6 decimals and rounded to 10 decimals (the column precision), so float noise never reaches storage or the UI.
- Margins are never clamped. Each has a status: `loss` (< 0), `below_target` (≥ 0 but under target), `meets_target`, or `unknown` (shown as N/A).
- Every token figure has a source: `actual_usage`, `history_model_effort` / `history_model` (at least 20 usage-log requests in 7 days), `admin_assumption`, `heuristic_char_count` (characters ÷ 4), or `insufficient_data`. Overall `estimate_quality` is the weakest of input and output. With `insufficient_data` (no pricing, no usable output history, or zero tokens) no margin is produced.
- Input tokens exclude cached input, as in usage logs. Reasoning tokens are billed as output and are captured by per-effort output history.
- `effort_multiplier` mirrors the billing engine's per-effort price multiplier from the pricing entry (1 when absent). `customer_rate` is 1 (list) for stored estimates; the decision detail re-estimates with the observed request's rate (charged ÷ list) and real input/cached tokens.
- Channel costs are shadow-only overrides per model. They do not change channel pricing, account rate multipliers or customer charges.
- Jev confidence values are shown as reported. They are not calibrated probabilities that a model will succeed.

The decision detail separates:

| Figure | Source |
|---|---|
| **Actual production** (served model, effort, tokens, charged amount, list cost, upstream account cost) plus observed model and fully loaded margins | The request's real `usage_logs` row, looked up at display time. |
| **Hypothetical recommendation** (A, B, C above) | Always labelled hypothetical, with estimate sources. |
| **Re-estimate with real input** | The recommendation priced with the observed request's input tokens and rate; output stays an estimate because the recommended model was never called. |

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
| POST | `/test` | Test lab: calls Jev only, stores nothing. Optional `assumptions` {input_tokens, cached_input_tokens, output_tokens} |
| POST | `/test/compare` | A/B: same sample through saved and draft instructions (two budgeted Jev calls), stores nothing |
| GET / PUT | `/classifier` | Jev instructions, defaults, effective questions, history / validate and save (versioned, audited) |
| POST | `/classifier/preview` | Effective questions for a draft; no Jev call, nothing saved |
| POST | `/debug/state` | Exact Jev request and extraction metadata for a synthetic request; no Jev call, nothing saved, body not audited |
| POST | `/simulate` | Per-model hypothetical economics for unsaved edits |

## Database migration

`backend/migrations/279_gpt_auto_classifier_versioning.sql` is additive too: it creates `gpt_auto_classifier_configs` and adds `classifier_config_hash`, `est_model_gross_margin`, `model_margin_below_target` and `estimate_quality` to the decisions table (existing rows keep defaults).

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
