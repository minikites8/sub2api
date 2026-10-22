# GPT Auto — Classification Context Diagnostics Report

Date: 2026-10-22. Base revision: `7e1a78597` (`main`, pushed, state-v3 / jev-questions-v2). All changes in this report are uncommitted working-tree changes; nothing was committed, deployed or enabled.

## 0. Starting point and version assumptions

- The working tree was clean at `7e1a78597`. state-v3 and jev-questions-v2 are committed and pushed in that revision. They are **not known to be deployed**: no production access was used. Until a deploy happens, production runs whatever build was deployed earlier (state-v1 at the last check).
- The running version can only be verified on a deployed node via `GET /api/v1/admin/gpt-auto/meta` (see §12). The frontend build or the Git branch is never used to infer it.

## 1. Implementation summary

Diagnostics are produced inside the existing state-v3 builder (`gptAutoAssembleContext`), from the same selection that builds the Jev state. There is no second parser. The builder now records, as it selects:

- an **item account** for every request item;
- each **selection step** in the order it runs, with status and characters charged;
- **follow-up resolution**, **agent activity counts** and **diagnostic notes**;
- a **completeness label**, which is a documented deterministic heuristic.

The shadow worker adds the length of the serialized Jev state. The object is stored in the existing `features` JSONB column as `features.context.diagnostics`. There is no new table and no migration.

What is new for administrators:

- The decision dialog has a **Classification Context Diagnostics** section with five parts:
  - a summary, including versions and the classifier hash;
  - a budget bar;
  - the selection timeline;
  - the item account and warnings;
  - a Jev-vs-policy comparison.
- The Test Lab state debugger has a **Context Extraction Preview**. It reuses the same component, adds the selected excerpts parsed from the server-built state, and shows the question criteria. There are eight synthetic fixtures.

What did not change:

- The Jev state did not change, except for two edge-case fixes (§7). A golden comparison of 60 synthetic fixtures (5 budgets × 2 strategies × 6 request shapes) produced byte-identical states before and after the change.
- Customer routing, effort, upstream selection, billing, responses and SSE are untouched.
- No extra Jev call is made.
- The request path has no new synchronous I/O.

## 2. Files added and modified

Added:

| File | Purpose |
|---|---|
| `backend/internal/service/gpt_auto_context_diagnostics.go` | Schema types, codes, completeness heuristic, index set |
| `backend/internal/service/gpt_auto_context_diagnostics_test.go` | 14 tests (extraction, metadata, security, persistence, debugger, logs) |
| `frontend/src/components/admin/gptAuto/GPTAutoContextDiagnostics.vue` | Diagnostics section (summary, budget bar, timeline, items, warnings, comparison) |
| `frontend/src/components/admin/gptAuto/__tests__/GPTAutoContextDiagnostics.spec.ts` | 12 component, helper and locale-coverage tests (real en/zh messages) |
| `frontend/src/utils/gptAutoContextDiagnostics.ts` | View helpers: availability, budget segments, timeline, warnings |
| `docs/GPT_AUTO_CONTEXT_DIAGNOSTICS_REPORT.md` | This report |

Modified:

| File | Change |
|---|---|
| `backend/internal/service/gpt_auto_context.go` | Scan counters. `take` reports included / truncated / budget. Activity trace (source items of events). Selection steps. Diagnostics assembly. Skipped-scan diagnostics. |
| `backend/internal/service/gpt_auto_shadow_service.go` | Worker records `state_chars`. Queue byte estimate covers diagnostics (fixed part 512 → 2048 bytes). |
| `backend/internal/service/gpt_auto_debug.go` | Debugger diagnostics carry `state_chars` |
| `backend/internal/handler/admin/gpt_auto_handler.go` | `GET /meta` adds `diagnostics_version` |
| `backend/internal/handler/admin/gpt_auto_handler_test.go` | Meta versions; debugger returns diagnostics |
| `backend/internal/server/middleware/audit_log_test.go` | GPT Auto sample routes omit bodies |
| `backend/internal/repository/gpt_auto_shadow_repo_integration_test.go` | JSONB round trip; legacy rows; no request text stored |
| `frontend/src/api/admin/gptAuto.ts` | Typed diagnostics interfaces; `diagnostics_version` in meta |
| `frontend/src/components/admin/gptAuto/GPTAutoDecisionLog.vue` | Uses the diagnostics section (replaces the one-line context summary); loads runtime versions from `/meta` |
| `frontend/src/components/admin/gptAuto/GPTAutoStateDebugger.vue` | Context Extraction Preview, excerpts, criteria |
| `frontend/src/utils/gptAutoDebugExamples.ts` | Eight synthetic fixtures |
| `frontend/src/i18n/locales/{en,zh}/gptAuto.ts` | `diagnostics.*`, debugger preview and fixture labels; removed three unused keys |
| `frontend/src/views/admin/__tests__/GPTAutoView.spec.ts` | Decision-log and debugger integration tests |
| `docs/gpt-auto-shadow-routing.md`, `.gitignore` | Docs section; report whitelisted |

## 3. Diagnostic schema (`context-diagnostics-v1`)

Stored at `features.context.diagnostics`, next to the existing state-v3 extraction metadata in `features.context`. That metadata is reused rather than duplicated: `state_version`, `strategy`, `items_total`, `budget_chars`, `used_chars`, `section_chars`, `limitations`, indices and turn counts.

```json
{
  "diagnostics_version": "context-diagnostics-v1",
  "scan_status": "scanned",
  "items": {
    "eligible": 1205, "represented": 9, "selected_messages": 4,
    "budget_dropped": 0, "budget_truncated": 1, "not_selected": 1196,
    "privacy": 2, "unsupported": 0, "external_references": 0, "compacted": 0, "empty": 0,
    "with_images": 0, "conversation_length_known": true
  },
  "selection": [
    {"section": "latest_user_request", "status": "included", "chars": 8, "items": 1},
    {"section": "continues_request", "status": "truncated", "chars": 1000, "items": 1},
    {"section": "agent_activity", "status": "included", "chars": 600, "items": 5},
    {"section": "original_request", "status": "not_applicable", "chars": 0},
    {"section": "history_summary", "status": "not_applicable", "chars": 0},
    {"section": "recent_context", "status": "included", "chars": 1400, "items": 3, "available": 60},
    {"section": "external_history", "status": "not_applicable", "chars": 0}
  ],
  "context_truncated": true,
  "follow_up": {"resolution": "linked_request", "short_message": true,
                "original_task_available": true, "agent_activity_available": true},
  "agent": {"scope": "before_follow_up", "step_limit": 8, "steps_detected": 12,
            "tool_calls_detected": 6, "tool_results_detected": 6, "tool_failures_detected": 2,
            "trailing_failures": 1, "failure_summaries_included": 2, "events_dropped_for_budget": 0,
            "event_kinds": {"tool_call": 3, "tool_result_error": 2},
            "has_tool_result_context": true, "has_progress_note": false},
  "notes": [],
  "completeness": "partial",
  "completeness_reasons": ["earlier_context_omitted"],
  "state_chars": 3218
}
```

**Invariants.** Both are enforced by tests on every extraction fixture:

- `items_total = eligible + privacy + unsupported + external_references + compacted + empty`.
- `eligible = represented + budget_dropped + not_selected`.
- The sum of step `chars` equals `used_chars`, and `used_chars ≤ budget_chars`.

**Unknown vs zero.** When a body over 16 MiB is not scanned, `scan_status` is `skipped_too_large`, and `items`, `selection` and `agent` are absent rather than zero. The completeness label is then `unable_to_determine`. In practice such requests are counted as `skipped_too_large` and never persisted. A Responses request whose history is held by the provider has `conversation_length_known: false`.

**Selection statuses:**

| Status | Meaning |
|---|---|
| `included` | Forwarded in full |
| `truncated` | Forwarded, but shortened or partly dropped |
| `budget_exhausted` | Present, but no budget was left |
| `unavailable` | Not contained in the request (provider-held or compacted history; a follow-up with no earlier request) |
| `not_applicable` | Nothing of this kind exists |

**Follow-up resolutions:**

| Resolution | Meaning |
|---|---|
| `not_follow_up` | The latest message is not a follow-up |
| `linked_request` | A continued request was forwarded |
| `agent_activity_only` | No earlier instruction, but the agent's previous attempt was forwarded |
| `unresolved` | Neither was available |
| `recent_turns_only` | The `recent_turns` strategy does not link follow-ups |

`short_message` together with `not_follow_up` marks a short message that the detector did not treat as a continuation. The follow-up text itself is never stored.

**Section 7 distinctions** (why something did not reach Jev):

| Category | Field |
|---|---|
| Not present in the request | `external_history` step `unavailable`; `history_by_reference`, `item_references`, `history_compacted`; `conversation_length_known: false` |
| Excluded for privacy | `items.privacy`: system/developer turns, harness messages, reasoning items. Raw tool outputs are summarised (`tool_outputs_summarized`). |
| Excluded by budget | `items.budget_dropped`, `items.budget_truncated`, step `budget_exhausted` or `truncated`, `agent.events_dropped_for_budget` |
| Unsupported | `items.unsupported` (unknown types or roles), `items.with_images` (images never forwarded) |
| Available but not selected | `items.not_selected`: outside the turn window, the 64-item retained tail, or the agent step limit |

**Completeness heuristic** (`gptAutoCompleteness`). It is deterministic. It is not a probability and not a measure of whether Jev understood the request; the UI says so next to the label.

1. `unable_to_determine`: the request was not scanned, or nothing in it is classifiable.
2. `important_unavailable`: `no_user_message`, or `follow_up_unresolved`.
3. `partial`: any of the following:
   - `latest_user_truncated`, `history_by_reference`, `item_references`, `history_compacted`, `earlier_context_omitted`, `images_not_forwarded`;
   - the notes `context_budget_exhausted`, `agent_events_trimmed`, `unsupported_items`.
4. `sufficient`: otherwise. Intentional exclusions (system prompts, summarised tool outputs) do not lower completeness.

`completeness_reasons` lists the codes that triggered the label.

**Diagnostic notes** are codes that Jev is not told about: `unsupported_items`, `empty_user_messages`, `context_budget_exhausted`, `agent_events_trimmed`, `follow_up_unresolved`, `no_classifiable_content`. The state's `context_limitations` are unchanged.

**Size.** Measured from marshalled JSON:

| Request | Diagnostics | Whole `features.context` |
|---|---|---|
| 600-item agent session | 1,401 bytes | 1,926 bytes |
| Plain one-message chat | 984 bytes | — |

A test caps `features` at 4 KB for an adversarial request that has 300 distinct tool names, secrets and injection text.

## 4. Data retention and privacy

**What is stored.** Diagnostics contain integers, booleans and codes from fixed vocabularies only. There is:

- no request text, follow-up text or Jev state;
- no tool names, arguments, outputs, file names or URLs;
- no credentials or headers.

Every list is bounded by its vocabulary: at most 7 selection steps, 5 event kinds, 9 completeness reasons and 6 notes. `TestGPTAutoDiagnosticsAreBoundedAndContentFree` and `TestGPTAutoDiagnosticsAgentToolFailures` assert this. The repository integration test reads `features::text` back from PostgreSQL and asserts that no request text is present.

**Retention.** Rows follow the existing decision retention (`DeleteOlderThan`); there is no separate store.

**Historical decisions.** Rows written before diagnostics existed have no `diagnostics` object. The UI then shows "Diagnostics unavailable — created before context diagnostics were enabled." Rows with no context metadata at all (state v1/v2) get an extra line saying so. Nothing is reconstructed or estimated.

**Synthetic debugger:**

- It requires admin authorization (route test).
- The body is excluded from the audit log (`auditBodyOmittedRoutes`, now also asserted by `TestGPTAutoSampleRoutesOmitBodies`).
- Nothing is stored (`TestGPTAutoDebugStateReturnsDiagnosticsFromTheSameBuild` asserts no repository write and no Jev call).
- Nothing reaches the application log: `TestGPTAutoDebugStateLogsNothingFromTheRequest` captures `slog` on both success and invalid input.
- The browser keeps the fixture in component state only: the spec asserts `Storage.prototype.setItem` is never called.
- The panel labels results "Administrator-provided test fixture".
- Existing redaction applies, and the 4 MiB debugger limit and 16 MiB scan limit are unchanged.
- There is no endpoint that returns the state of a real production request.

## 5. Backend API changes

| Endpoint | Change |
|---|---|
| `GET /api/v1/admin/gpt-auto/meta` | Adds `diagnostics_version` (alongside `state_version` and `classifier_schema`) |
| `GET /api/v1/admin/gpt-auto/decisions/:id` | `features.context.diagnostics` is present on new rows. Additive; no Jev call to read it. |
| `POST /api/v1/admin/gpt-auto/debug/state` | `context.diagnostics` is returned with `state_chars` set. Additive. |

No new endpoints. Authorization, audit, pagination and response envelopes are unchanged. Nothing is exposed through customer-facing routes.

## 6. Frontend components

`GPTAutoContextDiagnostics.vue` is used by both the decision dialog and the debugger. It has five parts.

**A. Summary.** Shows:

- strategy, state version and classifier schema;
- the abbreviated classifier hash and the diagnostics version;
- items scanned, messages selected and agent steps;
- follow-up resolution, the latest instruction, and whether history is in the request or held by the provider;
- truncation, characters used against the budget, and state characters.

If the state version recorded on the decision differs from the version that `/meta` reports for the node serving the page, a neutral note says so.

**B. Budget bar.** A stacked bar of what the builder charged to each part:

- latest instruction;
- continued request;
- original task and summary;
- agent activity;
- recent conversation;
- unused budget.

It is built only from recorded step characters; without diagnostics it is not drawn. Colours have dark-mode variants.

**C. Timeline.** The builder's steps in their real order, with `not_applicable` steps hidden. Each shows status, characters, messages or events, available and dropped counts, and agent details including tool-failure counts.

**D. Item accounting and warnings.** The item account is shown with hover hints. Warnings are neutral and derived only from recorded facts:

- truncated;
- continues an earlier task;
- task unresolved;
- tool failures;
- previous response unavailable;
- external history unavailable;
- images not included;
- context may be insufficient;
- not scanned.

No wording claims Jev was wrong; a test asserts that.

**E. Classification vs routing policy.** A table compares Jev's model, effort and confidence with the policy's model and effort, plus complexity, category, the top candidate probabilities and route modifications. Policy changes are highlighted.

**Debugger.** The "Context Extraction Preview" adds the excerpts exactly as they appear in the server state (latest request, continued request, agent events, original request, summary, recent turns) and the criteria of each question.

**Fixtures** (synthetic only):

- simple;
- routine code change;
- complex engineering;
- follow-up "continue";
- agent session with failing tools;
- 1,202-message chat;
- unresolved `previous_response_id`;
- image not available to Jev.

The layout uses responsive grids (`grid-cols-2 sm:grid-cols-3 lg:grid-cols-4`) and horizontal scroll for the table. Both locales (en, zh) are complete: the static key check passes, and a spec checks every dynamic code in both locales.

## 7. Context budget accounting

- Step `chars` are the characters the builder charged to the budget (`section_chars`).
- Text sections are charged their redacted, trimmed text length.
- Agent activity is charged summary + tool name + 24 per event, which is the existing allocation rule. The UI states this.
- Unused = `budget_chars − used_chars`.
- No quota or priority was changed. The shares are as before: latest message 50% (25% for a follow-up, 75% for `recent_turns`), continued request 25%, activity 25%, original request and summary 15% each, recent turns the rest.

**Two edge-case corrections to the builder.** These are the only places the state can differ from before. Neither occurred in the 60-fixture golden comparison.

1. `latest_user_truncated` used to compare the redacted output with the *unredacted* input, so redaction that shortened a message (for example a phone number replaced by `[redacted]`) was reported to Jev as truncation. It now compares against the redacted text. Covered by `TestGPTAutoLatestUserTruncationIgnoresRedaction`.
2. When fewer characters remained than recent turns, a turn received a share of 0 and was forwarded as an empty `{"role":…,"text":""}`. Such turns are now dropped and counted as `budget_dropped`.

## 8. Version compatibility

- Each decision carries its own identity:
  - `features.context.state_version`;
  - `features.context.diagnostics.diagnostics_version`;
  - `classifier_config_hash`, whose stored snapshot carries `schema_version`.

  A cluster with mixed backend versions therefore stays interpretable per decision.
- A row with an empty hash is shown as `jev-questions-v1 (before versioning)`, consistent with the earlier versioning work. A hash whose snapshot is missing is shown as Unknown.
- **Limitation:** rows do not record which node or build wrote them. `/meta` reports the constants compiled into the node that served the admin request. It is not a cluster-wide view, and the UI words the mismatch note accordingly. The build `Version` string is not unique per commit, so it was not added.
- Old rows (no diagnostics, or no context at all) decode and render without errors: unit test, integration test and frontend spec.

## 9. Performance

Diagnostics add only O(1) counters per item during the existing single scan. The extra work after selection is bounded by the turn and step limits: an index set of at most about 55 entries and at most 7 steps. Measured with `go test -bench BenchmarkGPTAutoSnapshot -benchmem -count 3` on an Apple M5, before (`7e1a78597`) and after:

| Benchmark | Before (mean) | After (mean) | Allocation |
|---|---|---|---|
| 11 MB, 10,000-item agent session | 27.19 ms, 1.005 MB, 5,697 allocs | 28.17 ms (+3.6%), 1.007 MB, 5,732 allocs | +~2.5 KB, +35 allocs |
| 6,000-message chat | 10.28 ms, 1.506 MB, 9,277 allocs | 10.40 ms (+1.1%), 1.508 MB, 9,292 allocs | +~2.3 KB, +15 allocs |

The body is not parsed a second time. `state_chars` is computed in the shadow worker, off the request path, by re-serializing the already bounded snapshot; `buildGPTAutoClassifierState` is deterministic. The worker makes no additional Jev or model calls and the request path makes no new database or network calls. The 16 MiB limit, queue size, queue byte cap, per-node and cluster budgets are unchanged. The queue byte estimate per snapshot now includes the diagnostics (fixed part 512 → 2,048 bytes).

## 10. Tests executed and results

All commands below were run in this session.

| Suite | Result |
|---|---|
| New backend tests (`gpt_auto_context_diagnostics_test.go`, 14 tests) | Pass |
| `go test ./internal/service ./internal/handler/admin ./internal/server/middleware -run GPTAuto` | Pass |
| Same with `-race` | Pass |
| `go test ./...` working tree vs a `7e1a78597` worktree | Same 3 failing packages (`internal/server/routes`, `internal/service`, `migrations`) and the identical set of 25 failing top-level tests in both; none are GPT Auto tests. **No new failures.** |
| Repository integration (`-tags integration`, Docker, `-run GPTAuto`; the unrelated, already non-compiling `ops_repo_lookup_deleted_key_audit_integration_test.go` excluded via overlay, as before) | 6/6 pass, including fresh-schema and upgrade migration tests and the new diagnostics JSONB round trip |
| `golangci-lint` (service, handler/admin, middleware, repository) | No findings in GPT Auto files (the 40 remaining findings are in unrelated files) |
| `gofmt -l` on changed Go files | Clean |
| Jev state golden comparison (60 synthetic fixtures, HEAD vs working tree) | 0 differences |
| New frontend spec (`GPTAutoContextDiagnostics.spec.ts`, 12 tests) + `GPTAutoView.spec.ts` (2 new tests) + utils + i18n | Pass (93 tests across 14 files) |
| Full `vitest run`, working tree vs `7e1a78597` worktree | Baseline: 25 failed / 4,223 passed. Now: 25 failed / 4,237 passed. **Identical failing-test set**; none are GPT Auto. |
| `vue-tsc --noEmit` and `vue-tsc -b` | Pass |
| ESLint on changed frontend files | No findings |
| `check:i18n` (locale key completeness) | Pass |
| `vite build` | Pass |

Coverage against the brief's test list:

- **Extraction:** standard conversation; short follow-up (`continue`, `fix it`, `try again`); continued-task resolution; tool execution and failures; more than 512 and more than 1,200 items; multimodal; missing external history (`previous_response_id`, item references, compaction); budget truncation and drops; agent events trimmed; empty user messages; unsupported items; oversized requests.
- **Metadata:** item invariants; budget accounting; agent counts; follow-up flags; limitation codes; unknown-not-zero; schema version persisted; historical rows.
- **Security:**
  - no prompts, tool output or secrets in stored rows (unit and PostgreSQL);
  - admin-only routes;
  - audit body omission;
  - no logging of the debugger body;
  - injection text cannot set diagnostics fields;
  - diagnostics never appear in the Jev state.
- **Frontend:** summary, budget bar widths, timeline order, warnings, historical states, theme classes, responsive classes, Chinese rendering, version note, debugger preview and excerpts, decision dialog, no browser storage.
- **Regression:** `TestGPTAutoShadowPipelineOnlyWritesShadowTables` and `TestGPTAutoObserveDoesNotMutateRequestBody` still pass. The request body, customer routing and billing are unchanged.

**Not run in this round:** a live browser walkthrough against a running stack, and any live Jev call. No real credentials and no customer data were used.

## 11. Remaining limitations

- Agent activity `chars` include the fixed 24-character per-event allowance used by the existing budget rule. These are "budget charged", not literal state characters. `state_chars` gives the literal total.
- `recent_context.available` counts candidates in the 64-item retained tail, not the whole conversation. Older items appear as `not_selected`.
- Images are detected only in user messages. Images inside tool outputs are not counted.
- Diagnostics do not record which node or build wrote a decision (§8).
- `/meta` is node-local.
- The completeness label is a heuristic. A `partial` label for a long but self-contained request is expected; the reasons list shows why.
- Diagnostics explain what Jev saw. They cannot establish *why* Jev chose a model. A Sol recommendation with `sufficient` context is not evidence of a classification error, and one with `partial` context is not proof that missing context caused it.

## 12. Deployment verification procedure

This needs explicit authorization; nothing here was executed against production.

1. Deploy a build containing these changes to staging first. Run the migrations; none are new, the latest remains 279.
2. On each node (or via the load balancer, repeatedly), call `GET /api/v1/admin/gpt-auto/meta`. Expect `state_version: "state-v3"`, `classifier_schema: "jev-questions-v2"` and `diagnostics_version: "context-diagnostics-v1"`. A node that omits `diagnostics_version` is running an older build.
3. In Test Lab → State debugger, run each synthetic fixture and check that the Context Extraction Preview matches expectations. For example:
   - "continue" → follow-up · earlier request linked;
   - unresolved `previous_response_id` → provider-held history, partial;
   - 1,202-item chat → items scanned 1,202.
4. Do not change shadow sampling as part of this deploy. If sampling is already enabled by an operator, open a new decision and confirm that the Classification Context Diagnostics section appears, and that older decisions show "Diagnostics unavailable — created before context diagnostics were enabled."
5. Optionally, query `SELECT features->'context'->'diagnostics'->>'diagnostics_version', count(*) FROM gpt_auto_shadow_decisions GROUP BY 1;` to see how many decisions have diagnostics, by diagnostics version.
