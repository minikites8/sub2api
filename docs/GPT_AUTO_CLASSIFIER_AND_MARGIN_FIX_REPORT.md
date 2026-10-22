# GPT Auto Phase 1: Jev Classifier Instructions, Routing Bias and Margin Fixes

**Date:** 2026-10-22. **Base revision:** `5de4efe9f` (`main`).

- Nothing was committed, pushed or deployed.
- Shadow sampling was not enabled anywhere except in a disposable local stack.
- Live `gpt-auto` routing was not implemented. Customer requests and billing are unchanged.

## 1. Why classifications favour GPT-6.1 Sol

The router is not hard-coded to Sol. The policy engine only changes Jev's model choice for three reasons:

- a hard constraint fails (the model is disabled, or the endpoint or tools are unsupported);
- Jev returns a model outside the pool;
- there is a near-tie and another model has a higher routing weight.

With the default weights a near-tie favours `gpt-6-luna` (100) over `gpt-6.1-sol` (90). So the Sol preference in production comes from Jev's own answer.

The causes below come from reading the v1 question set and the state construction. **They are not verified against live Jev**: no authorized TypeSafe credential was available. Production shadow logs and the opt-in live benchmark (§8) are how to confirm them.

| # | Cause in v1 | Effect |
|---|---|---|
| 1 | The model instruction said "choose the model **most likely to complete this task successfully**". | It contains no sufficiency criterion. The most capable model always maximises success probability, so any task that is not trivial favours the strongest model. For "hi" every model succeeds, which is consistent with the Test Lab result (Luna, complexity 0). |
| 2 | The default model descriptions framed GPT-6.1 Sol as "the strongest option in the pool, for the hardest tasks" and every other model as "the lighter option". GPT-6.1 Sol's description also mentioned effort ("does not run with reasoning disabled"). | This is a capability ranking, not a fit description, so it reinforces cause 1. It also couples model choice with effort. |
| 3 | The complexity rubric gave no coding example below level 2: "a typical bug fix" was Moderate (2) and "non-trivial code" was Complex (3). | Gateway traffic is mostly coding clients, so coding requests were anchored to 2–3. That matches the reported 2.2–2.75 band. |
| 4 | The state sent `approx_input_tokens` computed over the whole request. | This invites length-based judgements, and agent sessions are long. |
| 5 | Context selection included **system/developer turns** (agent and IDE harness prompts such as "You are an autonomous senior software engineer…"). | The text is engineering-heavy whatever the user actually asked. That pushes category towards coding and pushes complexity and model upward. |
| 6 | Turns after the latest user message were dropped without any signal. | Jev could not tell a fresh question from step N of an agent loop. |
| 7 | The effort question had no "decide independently of the model" instruction, and the model description mentioned effort. | This causes model/effort coupling (minor). |

Other suspects were checked and ruled out or fixed:

- **Requested model and effort:** never sent to Jev, in v1 or v2.
- **Model names:** the choice keys are the production ids (`gpt-6.1-sol` and so on). They stay, because stored data and answer validation depend on them. The v2 instruction tells Jev to judge GPT-5.6 models by described fit, not by generation number.
- **Probability handling:** the argmax `choice` is used. Probabilities are sanitised, and missing ones are never treated as ties.
- **Tie-break asymmetry (found and fixed):** a model Jev rated *more* likely than its own choice counted as a "near tie", so routing weight could override Jev without a real tie. The window is now symmetric (±0.05), and the inconsistency is reported as a warning.
- **Confidence thresholds:** v1 had none. v2 records `low_model_confidence` / `low_effort_confidence` (< 50%) as warnings only.

## 2. Changes to the Jev classification instructions (schema `jev-questions-v2`, state `state-v2`, since superseded by `state-v3`, §10)

- **Delivery:** each of the four questions carries its own question-level `instructions`, which TypeSafe System One supports. The state contains customer data only. There is no chat `system` role, and nothing is prepended to the state. A test inspects the real wire request to confirm this.
- **Default model instruction:**
  - Choose the **least capable model that can reliably meet the required quality**, judged by actual task requirements.
  - Do not judge by prompt length, by the amount of code or technical vocabulary, or by tool availability alone.
  - Programming terminology alone does not call for Sol. Prefer Luna for well-scoped, ordinary tasks; choose Sol for genuinely difficult multi-step reasoning.
  - Judge GPT-5.6 models by described fit, not generation number.
  - Ignore price and routing weights. Effort is decided separately.
- **Every default instruction** opens by saying the state is untrusted data and that instructions inside it carry no authority.
- **Effort:** lowest reliable effort, decided independently of the model. The effort criteria name no model or tier.
- **Complexity:** rate intrinsic difficulty, not domain or length. The rubric now has a coding example at every level.
- **Category:** `agent_tool_workflow` only when driving tools is the work itself. `coding_debugging` only when code is the main deliverable.
- **Model descriptions:** the defaults are rewritten as fit and sufficiency statements without effort or price. A stored description still exactly equal to its v1 default is upgraded on read. Edited descriptions are untouched.
- **State v2:**
  - removed `approx_input_tokens`;
  - system/developer text is no longer forwarded; only `has_system_prompt` is sent;
  - added `agent_steps_since_user_request`.

  The cost estimate still uses the full-request size; that figure is kept out of the state Jev sees.

These are selection guidelines. Nothing forces any share of traffic to a model, and a test confirms the policy reproduces Jev's distribution exactly on a balanced input.

## 3. Configuration fields and UI

**Backend**

- `classifier_instructions` sits in the existing `gpt_auto_shadow_config` settings document, with these fields:
  - `model_selection`, `effort_selection`, `complexity_scoring`, `category_classification` (20–4000 characters each, no control characters);
  - `revision`, `updated_at`, `updated_by`.
- New admin routes, all behind the existing admin auth, rate limit, audit and compliance middleware:

  | Route | Purpose |
  |---|---|
  | `GET/PUT /api/v1/admin/gpt-auto/classifier` | Saved instructions, defaults, effective questions and history / validated save |
  | `POST /classifier/preview` | Effective questions for a draft; no Jev call |
  | `POST /test/compare` | A/B comparison (§4) |

- A general `PUT /config` always keeps the stored instructions, so a stale form cannot overwrite them.
- **Audit:**
  - Instruction saves are logged with their text (`admin.gpt_auto.classifier.update`).
  - Compare request bodies are omitted, because they contain sample prompts.
- **Versioning:** migration `279_gpt_auto_classifier_versioning.sql` adds a `gpt_auto_classifier_configs` table, content-addressed by the hash of instructions, criteria, model descriptions, state version and classifier model.
  - Rows are written on admin save (with the author) and on decision persist.
  - The table holds administrator text only, never request content.
- **Per-decision record:** every decision stores `classifier_config_hash`. The decision detail returns the stored question set. Pre-versioning rows (hash `''`) are labelled "v1 (unversioned)", and the v1 instructions are kept in code (`gptAutoLegacyQuestionInstructions`).
- **No restart needed:** changes reach the next classification immediately on the saving node, and within the 30-second config cache on the others.

**UI**

- **New "Jev classification" tab:**
  - four independent textareas, each with a character counter and a per-field "Restore default";
  - "Restore all defaults", "Discard", "Preview effective questions" (shows every question with its criteria), and a save button separate from the config save;
  - the revision, hash, author and time;
  - a history table;
  - a note that model descriptions stay under Models & routing.
- **Decision log and detail:**
  - Jev's original recommendation and the policy-adjusted one are labelled separately.
  - Route modifications and warnings are separate lists, and "No route modifications" is stated explicitly.
  - The classifier version and its stored instructions are shown.
  - Replay states that it cannot evaluate changed instructions.
- **Overview:**
  - new charts for Jev raw effort and for decisions per classifier version;
  - a "Policy changed Jev's model" tile;
  - "Below target (model / fully loaded)".

## 4. A/B testing in the Test Lab

- **Turning it on:** tick "Compare saved and draft instructions (A/B)". The draft is pre-filled from the saved instructions.
- **Cost warning:** a banner says it makes **two separate Jev calls**, each counted against the daily call and cost limits and the concurrency limit.
- **How the calls run:**
  - The calls run one after the other, so a concurrency limit of 1 still works.
  - Each arm reserves and settles its own budget.
  - If the limit is reached after the first call, the second arm reports `GPT_AUTO_BUDGET_EXHAUSTED`.
- **Refusals:** a draft identical to the saved text is refused before any call. The draft is never saved or activated.
- **Output:** a side-by-side table of Jev model, policy-adjusted result, effort, complexity, category, model probabilities, confidence, latency, Jev tokens, Jev cost and classifier version. Changed values are highlighted, and the full detail of each arm can be expanded.
- **Also in the lab:**
  - synthetic benchmark presets;
  - workload assumptions (uncached input, cached input, output).
- **Separation:** policy-only replay stays separate and is labelled as unable to evaluate instruction changes.

## 5. Corrected financial semantics

All estimates go through one module, `gpt_auto_economics.go`, used by the shadow pipeline, the test lab, the simulator, replay and the decision detail.

```
list      = uncached_in × list_in + cached_in × list_cached + out × list_out
revenue   = list × billing_multiplier × effort_multiplier × customer_rate
upstream  = same tokens at the cheapest enabled channel (cached falls back to the channel input rate), else list
A  model gross margin        = (revenue − upstream) / revenue
B  fully loaded gross margin = (revenue − upstream − jev) / revenue
C  shadow classification overhead = actual Jev spend (billed tokens × Jev rate), never billed to customers
```

- **Arithmetic:** money is computed in `shopspring/decimal` from per-million rates rounded to 6 decimals, then rounded to 10 decimals (the column precision). The UI formatter removes float noise and shows about four significant digits, so `$0.09999999999999999` now displays as `$0.10` and `$0.00005292` stays exact.
- **Margin status:**
  - `loss`: negative (red);
  - `below_target`: positive but under the effective target (amber), e.g. 33.1% against a 43% target;
  - `meets_target` (green);
  - `unknown` (N/A).

  Negative margins are never clamped.
- **Estimate sources:** every token figure is labelled `actual_usage`, `history_model_effort` / `history_model` (at least 20 usage-log requests), `admin_assumption`, `heuristic_char_count` or `insufficient_data`.
  - `estimate_quality` is the weakest of the input and output sources.
  - When it is `insufficient_data`, no margin is produced or stored: the UI shows "N/A" and explains what is missing.
- **Effort and rates:** the billing engine's per-effort price multiplier is applied. Reasoning tokens are part of output.
- **Observed vs hypothetical:** the decision detail now shows:
  - **observed** production margins: charged amount vs recorded account cost, with and without Jev;
  - the recommendation **re-estimated** with the request's real input and cached tokens and its observed rate (charged ÷ list).

  Output stays an estimate there, because a Jev call never produces the recommended model's output.
- **Storage:** the existing columns `est_gross_margin` and `margin_below_target` keep their meaning (fully loaded). New columns: `est_model_gross_margin`, `model_margin_below_target`, `estimate_quality`.
- **Warning labels:** margin warnings moved out of route modifications. The misleading "margin below target → model" row no longer appears; legacy rows are re-labelled in the UI.

## 6. Why the -865.3% margin appeared, and the 1 / 10 token workload

The reported figures are arithmetically correct and are reproduced exactly by `TestGPTAutoReproducesReportedTinyRequestMargin`:

- **Jev cost:** 1,260 input tokens × $0.042/M = $0.00005292. Jev output is free.
- **Model margin:** ($0.000006 − $0.000005) / $0.000006 = **+16.7%**, which is positive but below target.
- **Fully loaded margin:** ($0.000006 − $0.000005 − $0.00005292) / $0.000006 = **−865.3%**.

The problem was presentation and inputs, not arithmetic:

1. **One margin mixed two costs.** It combined the model economics with the shadow-observation cost of the Jev call, and showed the result in red with no explanation. It is now split into A, B and C.
2. **The workload was a placeholder:**
   - **1 input token** was ⌈characters ÷ 4⌉ of the typed sample (for example "hi"). It ignores the system prompt, tools and history a real client sends.
   - **10 output tokens** was a usage-log average for that model and effort with no minimum sample count. A few tiny requests (test pings) were enough to produce it.

   Now:
   - history needs at least 20 samples, otherwise output is `insufficient_data` and the margin is N/A;
   - the sample-length heuristic is labelled "Heuristic estimate: not a reliable financial figure";
   - administrators can enter input, cached-input and output assumptions, labelled "Simulation assumption".
3. **Nothing distinguished the case.** A hypothetical tiny-request loss was shown the same way as an observed production loss. Every hypothetical panel now says it is not an observed production result, and observed figures are shown separately.

## 7. Test results

Every result below comes from a command actually executed on 2026-10-22.

| Check | Result |
|---|---|
| GPT Auto Go tests, service package (`-run GPTAuto`) | 68 passed, 1 skipped (live Jev benchmark, opt-in) |
| GPT Auto tests with `-race` (service, handler, admin, typesafe, server) | PASS |
| GPT Auto admin handler tests | 8 passed |
| Integration (`-tags integration`, real Postgres 18.1 + Redis 8.4 via testcontainers, with the pre-existing `-overlay` workaround) | 5 GPT Auto tests passed, including the migration upgrade (278 + 279) and the new classifier-config / decision-column test |
| Go regression: service, handler, repository, server, cmd/server, typesafe, migrations | 73 failing (sub)tests, **identical to the recorded baseline; 0 new** |
| `gofmt`, `go vet` | clean |
| golangci-lint v2.14.0 on touched packages | 0 findings in GPT Auto files (remaining findings are in untouched files) |
| Frontend `vue-tsc --noEmit`, `vue-tsc -b` | PASS |
| Critical vitest + GPT Auto specs + i18n completeness | 1,404 passed / 6 failed; the 6 are the same baseline failures (UsageTable, AppSidebar) |
| GPT Auto vitest (view spec + utils spec) | 22 passed |
| ESLint (full frontend) | 0 errors, 1 warning (pre-existing) |
| `vite build` (to a temp dir) | PASS |
| API E2E against a locally built server (disposable stack) | 32/32 PASS |
| Headless Chrome UI checks | 23/23 PASS, plus a 3-check economics rendering run |

**New Go tests** (`gpt_auto_classifier_test.go`, `gpt_auto_economics_test.go`, updated `gpt_auto_test.go`):

- **Instructions:** independent instruction fields; default guidelines content; validation limits; save, reload and reset with revisioning and author history.
- **Wire request:** the real System One request through an HTTP server stub. It confirms the request passes the gateway's System One validator, instructions sit only at question level, and there is no system prompt, requested effort or size field in the state.
- **Versioning:** versioned decisions and historical interpretability; the pipeline writes only shadow tables and leaves the request body untouched; legacy description upgrade.
- **Benchmark:** the synthetic benchmark is balanced; states carry no biasing signals; a harness system prompt does not change the state; the policy neutrality check; missing or uncertain probabilities.
- **Margins:** the -865.3% reproduction; 33.1% vs 43% reported as below target, not a loss; zero revenue; missing and thin history; cached-input pricing; monetary precision.
- **Test lab and A/B:** the test lab rejects the placeholder workload and accepts explicit assumptions; A/B makes two budgeted calls without activating the draft; A/B with a daily call limit of 1.
- **Other:** the output estimator minimum sample count; the actual-input re-estimate; observed economics.
- **Admin authorization:** all four new routes return 401 when unauthenticated; general config saves preserve the instructions.

**API E2E** used a fake TypeSafe key. Its 4 external calls (two A/B runs) were rejected by TypeSafe with `auth`, as intended. It confirmed:

- migration 279 applied;
- unauthenticated access rejected;
- defaults, server-side validation, preview not saving, revision bump, history with author, audited instruction text;
- config saves preserving instructions;
- simulator margins with no float artifacts;
- A/B making two Jev calls with +2 cluster-budget calls, the draft not activated, identical drafts refused, prompts and key absent from the audit log;
- versioned and legacy decisions readable, overview counts per classifier version;
- restore defaults returning the default hash.

## 8. Remaining limitations

- **Live Jev behaviour is NOT VERIFIED.** No authorized credential was available. Whether v2 reduces the Sol share has to be measured, using the procedure below.
- **Opt-in live benchmark:** `GPT_AUTO_LIVE_JEV_KEY=… go test ./internal/service -run TestGPTAutoLiveJevBenchmark -v` runs the 28 synthetic samples (19 expected Luna-sufficient, 9 Sol-warranted) and prints the distribution. It spends 28 Jev calls. The expectations are diagnostic labels, not ground truth.
- **Prompt cost:** the v2 question text is about 2.8 KB longer than v1 (6,367 vs 3,543 bytes of JSON, measured), so each Jev call carries roughly 700 more input tokens: about 2k instead of the reported 1,260 for a tiny sample. That is about $0.00008 per call at $0.042/M. The simulator's default Jev estimate was raised accordingly.
- **Model names:** the choice keys are still the production model ids, and JSON map order puts `gpt-6.1-sol` last. Position or name bias cannot be ruled out without live A/B data.
- **Stored estimates assume a 1× customer rate.** Only the detail view applies the observed rate. Channel-specific pricing, long-context multipliers and priority tiers are not modelled.
- **Input-token heuristic:** for production rows the input estimate is still ⌈characters ÷ 4⌉ of the full request (labelled heuristic) until the usage-log row exists.
- **Upgrading descriptions:** the v1 → v2 description upgrade changes the classifier hash on deploy even if nobody edits anything. This is intended, and the change is visible in the history.
- **Pre-existing, not fixed:**
  - the repository integration package still needs `-overlay` (`LookupDeletedKeyAudit` undefined);
  - the baseline Go and frontend test failures;
  - the onboarding tour on a fresh admin.

## 9. Recommended validation in production shadow mode

1. **Prepare.** Deploy with the default v2 instructions; no admin action is needed. Confirm the Jev classification tab shows revision 0 and a new hash, and that the overview shows a new classifier-version bar.
2. **Lab check.** Run the benchmark presets in the Test Lab on a handful of samples, or run the opt-in live benchmark once. Compare the tier agreement with the v1 behaviour you already observed.
3. **Shadow sample.** Keep the existing low sample rate. Use the "Decisions per classifier version" chart and the classifier version shown in each decision's detail to compare the old hash (or `''` for v1) with the new hash over a comparable window. Compare:
   - the share of `gpt-6.1-sol` in the "Jev raw model choice" chart;
   - the complexity and category distributions.
4. **Spot-check.** Open coding-agent decisions (`has_system_prompt`, `agent_steps_since_user_request` > 0). Check that routine edits land on Luna and genuinely hard tasks still get Sol.
5. **Iterate on drafts.** Before saving new instruction text, A/B it in the Test Lab on representative **synthetic** samples (never real customer messages). Watch the two-call cost in the result summary.
6. **Economics.** Judge profitability from the **model margin** and from the observed or actual-input figures in the decision detail. Treat the fully loaded margin as the Phase 2 question: is one Jev call per request affordable? Look at decisions marked "Insufficient data" or "Heuristic" before drawing conclusions.
7. **Not part of Phase 1.** Do not enable live routing or raise sampling based on these results.

## 10. Follow-up: conversation context extraction (`state-v3`)

### Deployment status (checked first)

`state-v2`, `state-v3` and `jev-questions-v2` exist only as uncommitted changes in the working tree.
- `origin/main` and `origin/feat/gpt-auto-shadow-phase1` both point at `5de4efe9f`, which still sends `approx_input_tokens` and the v1 questions.
- Production shadow results observed so far therefore come from v1. They must not be attributed to v2 or v3.
- After a deployment, `GET /api/v1/admin/gpt-auto/meta` reports the running `state_version` and `classifier_schema`. Each decision stores `classifier_config_hash` and `features.context.state_version`, so the version a result came from is visible.

### What changed

| # | Problem in v1/v2 | Fix |
|---|---|---|
| 1 | The scan read only the **first** 512 items. In longer conversations the latest user message and all recent activity were never seen; Jev classified an early message instead. | One linear pass over the whole array. Fixed-size bookkeeping is kept, and values are read as raw byte slices with an allocation-free member scanner (gjson would unescape every string). Only selected items are decoded, capped and redacted. A zero-copy body view is used, and an explicit detach clones every retained string. Bodies over 16 MiB are skipped (`skipped_too_large`). |
| 2 | Agent activity after the latest user message was reduced to a count. | `agent_activity`: steps, tool calls/results, failures, trailing failures, tools used, and the last N events (tool name and argument excerpt, result status with the relevant error lines, short progress notes). Bounded (240 characters per event) and redacted; tool outputs are never forwarded whole. Status detection inspects only the first 512 bytes and last 2 KB of each result. |
| 3 | A fixed "latest message + 4 previous turns" window. | `classification_context_strategy` (`task_aware` default, `recent_turns` = v2) and `classification_agent_steps` (1–30, default 8). The character budget is shared in priority order: latest instruction, continued request, current activity, original request and client summary, then recent turns. |
| 4 | "continue", "fix it", "try again" were classified as standalone trivial tasks. | Short follow-ups are detected (English and Chinese patterns, plus short anaphoric messages) only when earlier context exists. `latest_user_request_kind: follow_up`. `task_context.continues_request` carries the last real instruction. If the agent had stopped, the activity shown is the attempt the user is reacting to. The default instructions tell Jev to classify the continued task. |
| 5 | Missing history was silent. | `context_limitations`: `history_by_reference`, `item_references`, `history_compacted`, `no_user_message`, `latest_user_truncated`, `earlier_context_omitted`, `tool_outputs_summarized`, `images_not_forwarded`. Nothing is invented. A request with only tool results (`previous_response_id`) is classified from its visible activity, with an empty `latest_user_request`. |
| 6 | Harness text could leak in as "user" messages. | System and developer text is never forwarded; `system_context` carries only presence, a size bucket and a coarse kind. Client-injected user messages (`<environment_context>`, AGENTS.md, system reminders) are counted as harness messages, never used as the request. |
| 7 | No way to see what Jev receives. | Admin-only Test Lab **state debugger** (`POST /debug/state`). It shows the exact System One request (state and questions) plus extraction metadata, for synthetic requests only. No Jev call, nothing stored, request body omitted from the audit log. Five built-in synthetic examples are included (one with 1,200+ items). |
| 9 | Outdated snapshot test. | `TestGPTAutoSnapshotIsBoundedAndRedacted` now asserts that system text is absent from the state. |

The test lab now renders its sample as a request body and uses exactly the live extractor.

Decision rows store the extraction metadata in `features.context` (counts, indices, budgets and limitations; no content). The decision detail shows it.

### Measured cost (this development machine, `go test -bench`)

| Request | Time per snapshot | Allocated |
|---|---|---|
| 11.4 MB Responses agent session, 10,000 items | 28 ms | 1.0 MB |
| 3.8 MB chat, 6,000 messages | 11 ms | 1.5 MB |

Before the allocation-free scanner, the same agent session took 96 ms and allocated 68 MB.

### Tests (all executed on 2026-10-22)

| Check | Result |
|---|---|
| New context tests (`gpt_auto_context_test.go`) | PASS |
| GPT Auto Go tests with `-race` (service, handler, admin, typesafe) | PASS |
| Go regression on the touched packages | 73 failing (sub)tests, identical to baseline, 0 new |
| Integration, real Postgres and Redis (5 GPT Auto tests) | PASS |
| golangci-lint | 0 findings in GPT Auto files |
| Frontend `vue-tsc --noEmit` / `-b`, ESLint, `vite build` | PASS |
| Critical vitest + GPT Auto specs + i18n | 1,407 passed / 6 failed (the same baseline failures) |
| API E2E against a local server (debugger, long request, follow-up, previous_response_id, config) | 12/12 PASS |

The new context tests cover:
- conversations over 512 items (3,000+ messages) and a 1,400-step agent loop;
- tool failures with secrets in the output, short follow-ups (7 phrasings, 3 negatives) and follow-up chains;
- `previous_response_id`, item references and server-side conversations, and compaction;
- multimodal requests, prompt injection (including JSON-breaking text), and harness messages;
- the `recent_turns` strategy, budgets with a 12 MB body, the no-shared-memory check, the oversize skip, config validation, and the debugger.

### Limitations

- Follow-up, harness and compaction detection are heuristics (fixed patterns and prefixes). Unknown phrasings are treated as instructions.
- A user who starts their own message with a harness marker such as `<environment_context>` hides it from the shadow classifier. This only affects shadow classification of their own request.
- An image-only user message with no context is not classified.
- Tool-result status is a heuristic. An explicit exit code wins; otherwise error keywords in the last 2 KB decide.
- The synchronous scan adds about 2.5 ms per MB to *sampled* requests, capped by the 16 MiB limit.
- Live Jev behaviour with state-v3 is still unverified: no authorized credential was available.
