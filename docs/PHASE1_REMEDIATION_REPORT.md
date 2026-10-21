# GPT Auto Phase 1 — Remediation Report

**Date:** 2026-10-21. **Base revision:** `7eb3843d7` (`main`). Nothing was committed, pushed or deployed. Shadow sampling was not enabled anywhere except in a disposable local test stack.

**Scope:** fixes to verified defects in the Phase 1 shadow-routing implementation. Phase 2 (live routing) was not started. Production behavior for customers is unchanged: the hook still only reads the request body.

## Defects found and fixed

| # | Priority | Defect | Root cause | Fix |
|---|---|---|---|---|
| D1 | P0 | **Request path could block on the database.** When a daily limit was configured, `Observe` called `usageToday`, which ran `SELECT COUNT/SUM` with a 2 s timeout **while holding a mutex**, about every 10 s. One sampled request per window, and every request queued behind the mutex, waited on the DB. | The budget cache was refreshed inline in the hot-path check. | `Observe` now does no I/O. It reads an atomically stored "last observed cluster usage" view (`budgetLikelyExhausted`). The authoritative check is an atomic reservation on the worker. A test asserts 200 `Observe` calls finish while every budget-store call blocks forever. |
| D2 | P0 | **Unbounded memory.** The queue was a 100,000-slot channel of 192-byte value structs, about 19 MB pre-allocated even when the feature was off. Nothing capped snapshot bytes: `queue_size` 100,000 × `classification_max_context_chars` 200,000 allowed tens of GB of queued text. | Queue bounded only by count. | Pointer channel (8 bytes per slot), plus a global 64 MiB cap on queued snapshot bytes (`gptAutoMaxQueuedBytes`), accounted on enqueue and dequeue. |
| D3 | P1 | **Daily call and cost budgets were not global.** Each node added its own unsynchronized delta to a DB count refreshed every 10 s, so N nodes could each overshoot by a refresh window plus in-flight calls. The persisted count also excluded calls that were never stored. | Budget derived from persisted rows plus per-process deltas. | New `GPTAutoBudgetStore`; production uses Redis (`repository/gpt_auto_budget_cache.go`). Every Jev attempt reserves its estimated cost with an atomic Lua script, then settles to the billed cost (or refunds a call never sent). Each day is seeded from persisted decisions if Redis has no key (first start or a flush). **Fails closed** when Redis is unreachable and a limit is set. |
| D4 | P1 | **Test lab bypassed budgets.** `RunTest` neither checked nor recorded usage, so repeated admin tests could exceed the daily cost limit without limit. | Budget logic lived only in the shadow pipeline. | `RunTest` reserves and settles against the same cluster budget. It returns 429 `GPT_AUTO_BUDGET_EXHAUSTED`, or 503 `GPT_AUTO_BUDGET_UNAVAILABLE` if the budget store is down. |
| D5 | P1 | **Billed Jev usage was lost.** A 2xx response with a missing or invalid answer recorded 0 tokens and $0, though TypeSafe charged for it. Usage was decoded into strict `int`, so `120.0` or `"7"` failed the whole response. Retries recorded only the final attempt. | Decoder returned nothing on answer errors; strict struct decoding. | `EvaluateQuestions` returns the billed `Model`/`Usage` alongside answer-level errors and decodes usage leniently (shared `systemOneTokenCount`). The service sums usage and cost over all attempts, including failed ones. |
| D6 | P1 | **Circuit breaker could wedge permanently.** A half-open probe admitted by `allow()` was never released when the call was skipped (concurrency wait timeout, panic), so all classification stopped until restart. A budget skip called `record(true)`, closing the breaker with no successful probe. | No release path for an unused probe. | `releaseProbe()` runs via `defer` on every early exit; skips no longer record outcomes. |
| D7 | P1 | **Routing weight could override Jev without evidence.** With no model probabilities (omitted or all invalid), every candidate counted as a near-tie at 0, so the highest weight replaced Jev's choice. | Missing probability treated as 0. | Tie-break requires probabilities Jev actually reported for both models; otherwise Jev's choice stands. |
| D8 | P1 | **Secret handling.** Sending the masked hint (`ts-k****1234`) back saved it as the literal key. Changing `classifier_base_url` kept the stored key, so an edited URL would receive the credential. The classifier could also be pointed at this gateway's own public URL (customer-billable endpoint). | Only empty/clear were special-cased. | Masked hint echoed back → keep stored key; other `****` values → 400. A host change requires re-entering the key. A host equal to `api_base_url` or `frontend_url` → 400. |
| D9 | P1 | **No actual production cost.** Decisions stored `ctxkey.RequestID`, but `usage_logs.request_id` is `client:<id>`/`local:<id>`, so a decision could not be joined to the real request. The dashboard showed only hypothetical figures. | ID format mismatch; no lookup. | Decisions store the usage-log request id (`resolveUsageBillingRequestID`, computed only for sampled requests). `GetDecision` attaches `actual_usage`: served model, effort, tokens, charged amount, list cost and account cost. The UI shows it in a separate "Actual production request" section. |
| D10 | P1/P2 | **Unbounded work on the request goroutine.** The snapshot copied full text of up to 512 turns and ran the redaction regexes over the entire latest message before trimming. `trimRunesKeepTail` converted the whole text to `[]rune`. A multi-MB prompt meant regex passes over megabytes. | Trim-after-redact on unbounded strings. | Each turn is capped at 2× the budget as it is read and copied (no aliasing of the body). Redaction runs only on the capped text, and the final head/tail trim always falls inside that window, so a secret split at the boundary cannot survive. Trimming slices by byte offset. A ~10 MB body is handled in well under 2 s under the race detector. |
| D11 | P2 | `legend` values in score answers may be strings, objects or arrays (OpenAPI 0.2.0); the decoder accepted only strings. | Strict type. | Decoded raw; non-strings rendered as compact JSON. |
| D12 | P2 | Retention cleanup used `context.Background()`, so `Stop` could wait up to 30 s per batch. | Wrong parent context. | Uses the cancellable service context. |
| D13 | P2 | The in-memory "snapshot already written" cache never expired. After a pruned or restored snapshot table, new decisions referenced missing snapshots and historical replay failed. | Permanent cache. | Entries re-upsert (idempotent) after 1 hour. |
| D14 | P2 | Skipped and dropped counters were per process only. | No aggregation. | Each node flushes counter deltas to Redis about every 10 s (and on shutdown). The overview shows all-node counters for the UTC day; per-node values remain labelled as such. |
| D15 | P2 | The GPT Auto page lacked the Smart Ops in-page tab bar every sibling page has. The earlier report's navigation path ("sidebar → Smart Ops → GPT Auto") was wrong: the sidebar never renders the Smart Ops group (pre-existing, affects all Smart Ops pages). | Missing component; pre-existing sidebar grouping. | `SmartOpsNav` added to the view. Docs now give the real paths: sidebar search, Smart Ops tab bar, direct URL. The sidebar grouping itself was **not** changed (unrelated, pre-existing). |

## Files changed in this remediation

**New**
- `backend/internal/service/gpt_auto_budget.go`: budget interface, in-memory store, reserve/settle, hot-path view, counter flush.
- `backend/internal/repository/gpt_auto_budget_cache.go`: Redis Lua implementation.
- `backend/internal/service/gpt_auto_acceptance_test.go`: 19 regression tests.
- `backend/internal/repository/gpt_auto_acceptance_integration_test.go`: upgrade migration, actual usage, pagination, atomic Redis budget.
- `backend/internal/pkg/typesafe/testdata/systemone_openapi_0.2.0_examples.json`: official request/response examples, extracted verbatim from `https://api.typesafe.ai/openapi.json`.
- `docs/PHASE1_ACCEPTANCE_REPORT.md`, `docs/PHASE1_REMEDIATION_REPORT.md`.

**Modified**

Backend:
- `internal/pkg/typesafe/classify.go`, `classify_test.go`
- `internal/service/gpt_auto_shadow_service.go`, `gpt_auto_classifier.go`, `gpt_auto_policy.go`, `gpt_auto_decision.go`, `gpt_auto_config.go`, `gpt_auto_test.go`
- `internal/repository/gpt_auto_shadow_repo.go`, `repository/wire.go`
- `internal/handler/gpt_auto_shadow_helper.go`
- `internal/handler/admin/gpt_auto_handler.go`, `gpt_auto_handler_test.go`
- `cmd/server/wire_gen.go`: regenerated; the diff is the two budget-store lines.

Frontend:
- `src/api/admin/gptAuto.ts`
- `src/components/admin/gptAuto/GPTAutoOverviewPanel.vue`, `GPTAutoDecisionDetail.vue`, `GPTAutoDecisionLog.vue`
- `src/views/admin/GPTAutoView.vue`, `src/views/admin/__tests__/GPTAutoView.spec.ts`
- `src/i18n/locales/{en,zh}/gptAuto.ts`

Docs and repo:
- `docs/gpt-auto-shadow-routing.md`
- `.gitignore`: whitelist for the two report files.

No migration was added or changed. The Redis budget keys are `gpt_auto:budget:<UTC day>` and `gpt_auto:counters:<UTC day>`, with a 3-day TTL.

## Regression results after remediation

Every result below comes from a command actually executed; the acceptance report gives the exact commands.

| Check | Result |
|---|---|
| GPT Auto Go tests with `-race` (service, typesafe, handler, admin) | PASS; service suite also PASS at `-count=5` |
| Integration (`-tags integration`, real Postgres 18.1 + Redis 8.4 via testcontainers) | PASS (4 GPT Auto tests) |
| Go regression on all touched packages vs clean `7eb3843d7` | 25 top-level failures both before and after; **identical set, 0 new** |
| golangci-lint v2.14.0 on touched packages | 0 findings in GPT Auto files (others pre-existing) |
| `go vet` (default and `integration` tags) | PASS |
| Frontend `vue-tsc --noEmit`, `vue-tsc -b` | PASS |
| Critical vitest + GPT Auto spec | 1393 passed / 6 failed; **same 6 failures as baseline** |
| ESLint, full repo | 0 errors, 1 warning (same as baseline) |
| `vite build` | PASS |
| API E2E against a locally built server | 44/44 PASS |
| Browser E2E (headless Chrome via DevTools protocol) | 31/31 PASS |

## Remaining unresolved problems

- **Live Jev classification quality is NOT VERIFIED.** No authorized TypeSafe credential was available. Transport, auth and error mapping were exercised against the real API with a fake key, and response decoding against the official fixtures.
- **Classifier concurrency is per node**, so the cluster total is nodes × `classifier_max_concurrency`. Documented, deliberately not changed.
- **Cost reservations use a pessimistic estimate (bytes ÷ 3).** A call is refused if the estimate would cross the cost limit, even when the billed cost would have fit. The cost limit cannot be overshot by more than the per-call estimation error, which is negligible at $0.042/M.
- **Model descriptions sent to Jev are still neutral defaults.** Operators must refine them before relying on recommendations.
- **Pre-existing, not fixed:**
  - The sidebar does not render the Smart Ops group.
  - 25 Go and 6 frontend baseline test failures.
  - The repository integration package does not compile at HEAD (`LookupDeletedKeyAudit` undefined); worked around with `go test -overlay`.
  - The TypeSafe key is stored unencrypted in the settings table, like the existing content-moderation key, so database backups contain it.
