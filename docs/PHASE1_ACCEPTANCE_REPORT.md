# GPT Auto Phase 1 — Acceptance Report

**Date:** 2026-10-21. **Base revision:** `7eb3843d7` on `main`, with uncommitted GPT Auto changes in the working tree.

**Reviewer role:** independent verification, security review and remediation. Nothing was committed, pushed, deployed or enabled in production.

Statuses: **PASS** (executed and assertions passed), **FAIL**, **BLOCKED** (external dependency unavailable), **NOT TESTED**.

## 1. Executive summary

The implementation does not change production behavior, but **it was not acceptable as delivered**. The audit found 15 defects (D1–D15 in the remediation report). The most serious were:

- **D1:** a database query (up to 2 s, under a mutex) on the request path whenever a daily limit was configured.
- **D2:** queue memory that configuration could grow to tens of GB.
- **D3/D4:** daily budgets that were neither cluster-wide nor applied to the test lab.
- **D5:** billed Jev usage lost on invalid responses.
- **D6:** a circuit breaker that could wedge permanently.
- **D7/D8:** routing weight overriding Jev when probabilities were missing, and several secret-handling gaps.

All 15 are fixed and covered by new tests. The full test suites, an API-level end-to-end run against a locally built server, and a real-browser run all pass. They show **no new regressions** against a clean checkout of the base revision.

The one gate that cannot be closed here is **live Jev classification quality**: no authorized TypeSafe credential was available. The protocol was verified against the official OpenAPI 0.2.0 fixtures and a real (rejected) call to the provider.

**Recommendation: GO for controlled production shadow testing**, conditional on the steps in section 12.

## 2. Actual implementation coverage

| Area | Implemented | Verified by |
|---|---|---|
| Gateway hook in 4 handlers (Chat Completions and Responses, both handler families), read-only | Yes | Code trace; unit tests; live E2E (identical customer response with shadow on and off) |
| Jev System One: 4 questions in one call (choice ×3, score ×1) | Yes | Official contract fixtures; schema validator test; real call with fake key |
| Capability matrix from gateway rules | Yes | Tests over all 4 models × 6 efforts |
| Policy engine, profiles, multipliers, weights, margins | Yes | Unit tests; replay tests |
| Hypothetical economics plus actual production usage | Yes; actual usage added in remediation | Unit and integration tests (real `usage_logs` join) |
| Bounded queue, workers, breaker, retries, shutdown | Yes (fixed) | Unit and race tests; live SIGTERM |
| Cluster-wide daily budget | Added in remediation (Redis) | Unit tests; real Redis integration test |
| Decision log, replay (historical/current/custom), retention | Yes | Integration, API E2E, browser E2E |
| Admin UI: overview, config, logs, test lab | Yes | Component tests; browser E2E |
| Migration 278 | Yes | Fresh install and upgrade on real Postgres |
| `gpt-auto` exposed to customers | No (correct) | `meta` and routes inspected; API E2E |

## 3. Verification matrix

| Requirement | Status | Evidence |
|---|---|---|
| Never changes requested model | PASS | The hook reads only. Tests: `TestObserveGPTAutoShadowDoesNotTouchResponseOrBody`, `TestGPTAutoObserveDoesNotMutateRequestBody`, `TestGPTAutoObserveBodyIsNotRetained`. E2E: decision records `actual_model=gpt-5.6-sol`; response unchanged. |
| Never changes reasoning effort | PASS | Same tests. Effort rewriting elsewhere produces new buffers. |
| Never changes account selection | PASS | The hook runs before slot acquisition and selection, and does not replace `c.Request` or its context. Code trace in all 4 handlers. |
| Never changes billing | PASS | No billing dependency. Jev cost exists only on decision rows and in the Redis budget. Test: `TestGPTAutoClassifierCostIsInternalOnly`. |
| Never modifies request body | PASS | Byte-for-byte comparison tests. The snapshot copies text (`strings.Clone`). |
| Never blocks main response on Jev | PASS | `TestGPTAutoClassifierFailureNeverBlocksObserve` |
| No synchronous I/O on hot path | PASS (after fix D1) | `TestGPTAutoObservePerformsNoIOOnHotPath` |
| Never corrupts SSE/streaming | PASS | The hook writes nothing to the response writer. Test asserts response untouched. |
| Never runs alternative GPT inference | PASS | The only outbound client is TypeSafe System One. |
| Never calls the customer-billable gateway | PASS | Direct TypeSafe client. A base URL matching this gateway's `api_base_url`/`frontend_url` is rejected (`TestGPTAutoConfigRejectsGatewayAsClassifier`). `/v1/systemone` is not observed. |
| Bounded memory | PASS (after fix D2) | `TestGPTAutoQueueMemoryIsBounded`, `TestGPTAutoSnapshotOfHugeBodyIsBoundedAndDetached` |
| No goroutine per request; no leaks | PASS | Fixed pool; `TestGPTAutoStopCancelsInflightCallsWithoutLeaks` |
| Graceful shutdown cancels in-flight Jev | PASS | Unit test; live SIGTERM: `[Cleanup] GPTAutoShadowService succeeded`, process exited in 3.2 s |
| Data races | PASS | `-race` on all GPT Auto tests, service suite ×5 |
| Jev request schema (choice/score criteria) | PASS | `TestGPTAutoQuestionsPassSystemOneValidation`; official fixture round-trip |
| Jev response decoding (choice, score, probabilities, confidence, legend) | PASS | `TestEvaluateQuestionsOfficialContractFixtures`; malformed/incomplete cases |
| Usage and cost accounting | PASS (after fix D5) | `TestGPTAutoBilledInvalidAnswerIsCosted`, `TestGPTAutoCostBudgetSettlesToBilledCost` |
| Auth/error handling against real provider | PASS (partial) | Fake key → real `api.typesafe.ai` → 401/403 → class `auth`. Body not leaked. |
| Live Jev classification with valid credentials | **BLOCKED** | No authorized credential |
| Migration: fresh database | PASS | Integration harness applies 1–278 on Postgres 18.1 |
| Migration: upgrade 277 → 278, re-run idempotent | PASS | `TestGPTAutoMigrationUpgradeFromPreviousSchema` |
| Insert / query / filter / paginate / replay / retention delete | PASS | Integration tests; API E2E |
| Global daily budget across instances | PASS (after fix D3) | `TestGPTAutoDailyBudgetIsGlobalAcrossNodes` (3 nodes), `TestGPTAutoBudgetStoreIsAtomicAcrossNodes` (real Redis: 8 concurrent clients, 160 attempts, exactly 25 admitted) |
| Budget fails closed when shared store is down | PASS | `TestGPTAutoBudgetFailsClosedWhenStoreUnavailable` |
| Manual tests cannot bypass budgets | PASS (after fix D4) | `TestGPTAutoRunTestHonoursBudget` |
| Admin-only access | PASS | 401 without or with an invalid token (E2E); route group behind admin auth, compliance guard, audit |
| Key never returned to frontend | PASS | API and browser E2E; handler tests |
| Masked key not saved literally; omitted key preserved; explicit replace/clear | PASS (after fix D8) | `TestGPTAutoConfigSecretHandling`; E2E |
| Key absent from logs and audit trail | PASS | E2E grep of server log and `audit_logs` |
| Dangerous config rejected server-side | PASS | Validation tests; E2E (private host, http, 6.1-sol + `none`, out-of-range values) |
| Snapshot redaction; no prompts in decision rows | PASS | Redaction tests; E2E (prompt and email absent from the stored row) |
| Policy: capability, effort correction, weights, multipliers, margins, all 4 models | PASS | Policy tests incl. `TestGPTAutoPolicyAllModelsAndEfforts`, `TestGPTAutoTieBreakRequiresReportedProbabilities` |
| Replay deterministic, never calls Jev, historical preserved | PASS | `TestGPTAutoReplayDoesNotCallClassifier`, `TestGPTAutoHistoricalReplayKeepsOldPolicy` |
| Actual vs hypothetical vs Jev cost vs unknown pricing distinguished | PASS (after fix D9) | `TestGPTAutoDecisionCarriesActualUsage`; UI sections and labels |
| Frontend: overview, model and routing config, logs, test lab | PASS | Component tests; browser E2E 31/31 |
| Frontend typecheck, lint, build, i18n | PASS | Section 4 |

## 4. Tests executed and exact results

**Baseline.** A clean worktree was created with `git worktree add --detach /tmp/gptauto-accept/baseline 7eb3843d7`.

**Tooling.** Go 1.27.3; Node v24.19.0 (the Codex-bundled binary, since no Node is on `PATH`); golangci-lint v2.14.0 installed to `/tmp`; Docker Desktop 29.9.1.

| # | Command (backend run from `backend/`) | Result |
|---|---|---|
| 1 | `go test -race -count=1 -run 'GPTAuto\|EvaluateQuestions\|ObserveGPTAuto\|SystemOne\|TypeSafe' ./internal/service/ ./internal/pkg/typesafe/ ./internal/handler/ ./internal/handler/admin/` | ok ×4 |
| 2 | `go test -race -count=5 -run GPTAuto ./internal/service/` | ok |
| 3 | `go test -count=1 -tags integration -overlay overlay.json -run GPTAuto -v ./internal/repository/` (overlay drops only the unrelated broken `ops_repo_lookup_deleted_key_audit_integration_test.go`) | 4/4 PASS: `RoundTrip`, `MigrationUpgradeFromPreviousSchema`, `ActualUsageAndPagination`, `BudgetStoreIsAtomicAcrossNodes` |
| 4 | `go test -count=1 ./internal/service/ ./internal/handler/... ./internal/repository/ ./internal/server/... ./cmd/server/ ./internal/pkg/typesafe/ ./migrations/...` (baseline and after) | Both: FAIL in `service`, `server/routes`, `migrations`, with 25 top-level failures (73 lines incl. subtests). `comm` diff of the sorted lists: **no new failures, none fixed**. All other packages ok. |
| 5 | `go vet` on touched packages; `go vet -tags integration` on `repository` | PASS |
| 6 | `golangci-lint run ./internal/service/... ./internal/handler/... ./internal/repository/... ./internal/pkg/typesafe/... ./cmd/server/...` | 0 findings in GPT Auto files. The two `ineffassign` hits in touched handler files reproduce on baseline. |
| 7 | `vue-tsc --noEmit`; `vue-tsc -b` | PASS |
| 8 | `vitest run <81 critical files from Makefile> src/views/admin/__tests__/GPTAutoView.spec.ts` | 1393 passed / 6 failed. Baseline (critical only): 1382 passed / 6 failed, identical failing tests. GPT Auto spec: 11/11. |
| 9 | `vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts` | 3/3 PASS |
| 10 | `eslint . --ext .vue,.js,...` (CI `lint:check`) | 0 errors, 1 warning (same warning on baseline) |
| 11 | `vite build --outDir /tmp/...` | PASS; GPTAutoView chunk 76.75 kB (19.04 kB gzip) |
| 12 | API E2E: `python3 /tmp/gptauto-accept/e2e_api.py` against the server built from the working tree, fresh Postgres 18.1 and Redis 8.4 containers, env auto-setup | 44/44 PASS |
| 13 | Browser E2E: `node /tmp/gptauto-accept/browser/e2e_browser.mjs` (headless Chrome over DevTools protocol, Vite dev server proxying to #12) | 31/31 PASS; screenshots in `/tmp/gptauto-accept/screens/` |
| 14 | Live SIGTERM of #12 | GPT Auto cleanup step succeeded; process exited in 3.2 s |

**Test environment notes:**
- The disposable stack's admin compliance guard was satisfied by seeding a clearly labelled **test-fixture** row directly in the throwaway database. No real acknowledgement was submitted.
- The throwaway user received a test balance so gateway requests reach the handler.
- All containers were stopped afterwards.

## 5. Baseline versus newly introduced failures

| Failure | Classification |
|---|---|
| 23 `internal/service` tests (ExcelBPS ×8, OpenAIWS ×2, Pricing, ModelBilling, ChannelMonitorV2, CNProvider, CodexBlacklist, ControlledExperiment, Pelican, PriorityCold, GrokMedia, InitialAdmission, RecordUsage multiplier, scheduler BPS cooldown) | Pre-existing (identical on `7eb3843d7`) |
| `TestGatewayRoutesPinnedModelsDispatchesOrdinaryAndCodexRequests` (`server/routes`) | Pre-existing |
| `TestCompositeRouteTargetPlatformFinalStateCoversAllPlatforms`, `TestUserPlatformQuotaPlatformCheckFinalStateCoversAllPlatforms` (`migrations`) | Pre-existing |
| 4 `UsageTable.spec.ts`, 2 `AppSidebar.spec.ts` (frontend) | Pre-existing |
| Repository integration package does not compile (`repo.LookupDeletedKeyAudit` undefined; test added in `13cd8c4fc`) | Pre-existing; isolated with `go test -overlay` |
| Docker Hub token fetch EOF on first pull | Environment (transient; retry succeeded) |
| Newly introduced by GPT Auto | **None** |

## 6. Security assessment

- **Authorization:** all `/api/v1/admin/gpt-auto/*` routes sit behind admin JWT, the compliance guard, the panel rate limit and the audit middleware. Unauthenticated and invalid-token requests get 401 (verified live).
- **Credential exposure:** the key is never returned (GET returns only a masked hint), never logged, and omitted from the audit log for `PUT /config` and `POST /test`. Verified by grepping a live server log and `audit_logs` for the key.
- **Credential misuse:** the remediation closes three paths:
  - A masked hint echoed back is ignored.
  - The stored key no longer follows a host change.
  - The classifier cannot point at this gateway's own host.

  Outbound calls require public HTTPS, and resolved IPs are re-checked against DNS rebinding.
- **Data minimization:** snapshots are redacted (credentials, emails, phone numbers) and truncated before leaving the gateway. Decision rows hold no prompt text (verified live). The test-lab prompt is not stored and is not in the audit log (verified live).
- **Resource abuse:** the queue is capped by count and bytes, budgets are cluster-wide and fail closed, test-lab calls count against budgets, and the circuit breaker can no longer wedge.
- **Residual risks:**
  - The TypeSafe key is stored unencrypted in `settings` (same as the existing content-moderation key), so database backups contain it.
  - Sampled, redacted text goes to a third party, which the admin UI discloses before enabling.
  - Redaction is pattern-based and cannot catch every kind of sensitive data.

## 7. Database verification

- **Syntax and ordering:** migration 278 follows 277 and is additive. It uses `SET LOCAL lock_timeout/statement_timeout` and `IF NOT EXISTS`. Applied on PostgreSQL 18.1, the only engine the runner supports (`lib/pq`, advisory locks).
- **Fresh install:** all migrations 1–278 applied (integration harness and the live server's auto-setup).
- **Upgrade:** 1–277 applied, data inserted, then 278 applied and the full set re-run. 278 was recorded once, existing data was intact, and all 6 indexes exist.
- **Types and keys:**
  - Costs are `NUMERIC(20,10)` (confirmed with `format_type`), and 1e-10 USD round-trips.
  - Margins and confidences are `DOUBLE PRECISION`.
  - `policy_snapshot_hash` is a soft reference with no foreign key, so retention deletes stay cheap. Snapshots are content-addressed and never deleted, and the service re-writes them if they go missing (fix D13).
- **JSON:** classification and decision JSONB round-trip, including probability maps (integration and E2E).
- **Pagination:** newest first, with no gaps or duplicates across 3 pages; totals and page counts are correct (integration). Invalid filters return 400 (E2E).
- **Retention:** batched `DeleteOlderThan` deletes exactly the batch size (integration). The hourly loop now uses a cancellable context.
- **Actual-usage join:** `usage_logs` lookup by `request_id` within ±6 h. It is index-backed and partition-prunable, and the account cost formula was verified (`total_cost × account_rate_multiplier`).

## 8. Jev API verification

- **Contract source:** `https://api.typesafe.ai/openapi.json`, TypeSafe API 0.2.0, fetched 2026-10-21. Its choice and score request/response examples are stored verbatim as a test fixture.
- **Request:**
  - `model` (`jev-latest`), string `state`, and named `questions`.
  - Choice criteria as a name→description map (4, 6 and 6 entries; limit 1–255).
  - Score criteria as an ordered array (5 levels; limit 1–10).
  - Bearer auth.
  - The fixture test re-serializes the official examples byte-for-byte (JSON-equal).
- **Response:**
  - `model`, `usage.{input_tokens,output_tokens}`, and `answers` keyed by question name.
  - Choice answers: `choice`, `confidence`, `probabilities`. Score answers: `score` (expected value; may be fractional), `confidence`, `legend`, `probabilities`.
  - The decoder rejects choices outside the offered criteria, missing or type-mismatched answers, NaN or Inf scores, and missing `model`.
  - Out-of-range confidence or probabilities degrade to 0 or are dropped rather than failing a billed answer.
- **Errors:**
  - 401/402/403 → `auth`; 429 → `rate_limited` (retried); 5xx → `upstream_error` (retried); 400/413/422 → `invalid_request`; 2xx with bad answers → `invalid_answer` (now with billed usage).
  - Timeouts are enforced per attempt by the service.
  - Provider bodies never appear in errors or logs.
- **Live:**
  - An unauthenticated GET `/v1/models` returned 403 `authentication_error`.
  - A test-lab call with a fake key reached the real endpoint and was classified `auth`.
  - **Classification with a valid key: BLOCKED** (no authorized credential). Answer semantics, real token counts and latency are therefore unverified.

## 9. Frontend acceptance results

Headless Chrome against the real backend and database, logged in through the real login form:

- **Overview:** backend data rendered (2 stored decisions, 1 success / 1 failed), status "Disabled", cluster counters panel and today's all-node budget. No "budget store unavailable" banner while Redis is up; the banner appears when it is down (component test).
- **Decision logs:** rows for success and failed decisions. The detail shows probabilities, the effort correction `none → low` (gpt-6.1-sol), and the actual-production section ("not found" for a synthetic row). Historical replay runs, and replay of a failed decision is refused (API).
- **Model & routing:**
  - All four profiles have independent multiplier and weight inputs.
  - The key field stays empty; `none` is disabled for gpt-6.1-sol.
  - An invalid multiplier disables Save.
  - Edits persisted independently (`gpt-6-luna` 1.7× / weight 100 unchanged; `gpt-5.6-sol` weight 321 / multiplier 1.4 unchanged) and reload after navigation. The stored key survived the UI save.
- **Test lab:** submitted to the backend, which called TypeSafe and showed the classified failure ("Jev classification failed: auth"). A successful classification display is covered by a component test only.
- **Navigation:** the Smart Ops tab bar on sibling pages links to GPT Auto (clicked and verified). At 390 px there is no horizontal scroll. No uncaught page errors.

**Not verified in the browser:** a successful live Jev result (blocked as above) and the dark theme.

## 10. Known limitations

- **Live Jev:** classification quality, token counts and latency are unverified.
- **Model descriptions:** the defaults sent to Jev are generic. Refine them before trusting recommendations.
- **Per-node limits:** classifier concurrency, queue and circuit breaker are per node, so cluster concurrency is nodes × limit.
- **Estimates:** input tokens for hypothetical economics are a character estimate (÷4). Output tokens come from 7-day `usage_logs` history and are excluded and labelled when there is no history. `cached_input_per_m` is stored but unused.
- **Capability nuance:** on compatible (non-official) upstream accounts the gateway strips `reasoning_effort:"none"`; the matrix is per model, not per channel.
- **Economics replay:** replay uses current pricing and output history, so the route is deterministic but economics can differ from the original row.
- **Navigation:** the sidebar does not render the Smart Ops group (pre-existing). The page is reachable via sidebar search (verified by code reading), the Smart Ops tab bar, or the URL.
- **Key storage:** the TypeSafe key is stored unencrypted in the settings table.

## 11. Outstanding blockers

| Blocker | Impact | Required action |
|---|---|---|
| No authorized TypeSafe credential | Live classification unverified | Before widening sampling, run 5–10 synthetic prompts in the Test Lab with the production key. Check that answers are plausible, latency is under the timeout, and the reported cost matches the overview. |
| None else for shadow mode | — | — |

## 12. Final recommendation

**GO** for controlled production shadow testing, under these conditions:

1. Deploy with defaults (disabled, 0 %). Confirm the overview shows "Disabled" and **no** "budget store unavailable" warning, which would mean Redis is unreachable.
2. Enter the Jev key and run the Test Lab check from section 11 before enabling sampling.
3. Replace the default model descriptions.
4. Start with one internal group, 1 % sampling, and low daily limits. The limits are enforced across all nodes.
5. Watch error rate, latency, breaker, drops and spend; widen gradually.

**NO-GO** for anything beyond shadow mode. Phase 2 live routing was not implemented and must not be inferred from this approval.
