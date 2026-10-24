# GPT Auto Phase 2.2 acceptance

Validation dates: 2026-10-24–25 (Asia/Shanghai). Branch: `main`. No commit, push or deployment performed. Existing unrelated untracked handler test and service data directory preserved.

## Environment and commands

macOS Apple M5 / arm64; existing `gpt-auto-local` Compose project. PostgreSQL, Redis, backend with embedded production Vue build, Nginx (`web`) and local Jev/upstream mock. All inference acceptance requests use `http://127.0.0.1:18080` through Nginx. Startup additionally checks direct backend health/assets. No production services changed.

Commands executed from the repository root unless otherwise shown:

```sh
git status --short
git branch --show-current
find .. -name AGENTS.md -print
rg --files -g '*gpt_auto*' -g '*GPTAuto*' -g '*gpt-auto*'
git diff --check

cd backend
go test -tags=unit ./internal/service ./internal/handler ./internal/repository -run 'TestGPTAuto|TestOpenAIHosted|TestCalculateOpenAIRecordUsageCost' -count=1
go test -tags=unit -race ./internal/service ./internal/repository -run 'TestGPTAutoDynamic|TestGPTAutoAffinityConcurrentCAS' -count=1
go test -tags=unit ./internal/service -run '^$' -bench '^BenchmarkGPTAutoDynamicSessionCAS$' -benchmem -count=1

cd ../frontend
export PATH=/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH
./node_modules/.bin/vitest run src/views/admin/__tests__/GPTAutoView.spec.ts src/views/admin/__tests__/GPTAutoPricing.spec.ts src/utils/__tests__/gptAuto.spec.ts src/components/user/__tests__/GPTAutoRoutingHistory.spec.ts src/components/admin/gptAuto/__tests__/GPTAutoContextDiagnostics.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/eslint src/components/admin/gptAuto/GPTAutoLivePanel.vue src/components/admin/gptAuto/GPTAutoDecisionLog.vue src/api/admin/gptAuto.ts src/views/admin/__tests__/GPTAutoView.spec.ts

cd ..
bash scripts/test-gpt-auto-local.sh build
bash scripts/test-gpt-auto-local.sh start
python3 scripts/gpt-auto-local/verify_phase22.py
python3 scripts/gpt-auto-local/verify_phase21.py
/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node scripts/gpt-auto-local/browser_phase22.mjs
```

Real-provider opt-in test: `scripts/gpt-auto-local/verify_phase22_real.py`, invoked through `runpy.run_path(..., run_name='__main__')` with `scripts/gpt-auto-local` on `sys.path` and `GPT_AUTO_TEST_JEV_KEY` injected into the Python process. The credential value is deliberately omitted from this report. It was not added to repository files or evidence; the script removes it from its environment and restores the synthetic provider plus routing OFF in `finally`.

A final-build automatic approval review timed out before execution. The explicitly allowed retry succeeded; this was not a compilation failure.

## Automated coverage

Backend selected service/handler/repository suites pass. Added scenarios cover per-turn upgrade, repeated downgrade, independent effort, downgrade candidate/task reset, flags, group-cap synchronization, hard model/account binding, missing state, permission revocation, Redis failure, CAS conflicts and expiration, capacity fallback respecting explicit switch controls, export append compatibility and customer isolation. Existing GPT Auto pricing, context, failure/budget/breaker, legacy routing, partial settlement and hosted-tool tests run in the same selection.

Frontend: **54 tests across six files pass**. Type checking and changed-component lint pass. Build runs locale-key completeness and production Vue/Go compilation. Existing chunk-size/Browserslist warnings are not introduced routing failures.

Race detection covers dynamic router and Redis CAS. Local CAS microbenchmark: **155.6 ns/op, 0 B/op, 0 allocs/op** on this run. This measures an existing-key in-memory update, not a complete gateway request, network classification or Redis process memory.

## Mock workflow expectations

One API key, one session header and one prompt-cache key; same meaningful task anchor with updated workflow context. Threshold is two consecutive recommendations for the same lower candidate:

| Turn | Raw Jev | Executed | Why |
|---|---|---|---|
| 1 | gpt-6-luna / low | gpt-6-luna / low | New session |
| 2 | gpt-6.1-sol / high | gpt-6.1-sol / high | Immediate upgrade |
| 3 | gpt-6.1-sol / medium | gpt-6.1-sol / medium | Effort only |
| 4 | gpt-6-luna / low | gpt-6.1-sol / low | First lower-candidate vote |
| 5 | gpt-5.6-luna / none | gpt-6.1-sol / low | Different lower candidate resets streak; Sol cannot use none |
| 6 | gpt-5.6-luna / none | gpt-5.6-luna / none | Second matching lower-candidate vote |

Every turn calls Jev and actual mock upstream model matches the decision. Alternating Chat SSE/JSON is valid; separate Chat/Responses SSE and tool tests also run. Every identical 1000 total input / 200 cached / 500 output usage settles once at **$0.0057** under the unchanged virtual tariff, with three different procurement amounts. No second classifier fee reaches the customer.

## Real Jev smoke test (authorized provider)

Provider: `https://apinode.ltd`, model `jev-latest`. Only four synthetic task prompts and synthetic history per run were sent; model execution remains on the local mock. No customer data or repository code was sent. This is integration evidence, **not an answer-quality benchmark**.

First run: one initial transport error returned structured 503 `gpt_auto_routing_unavailable`; the remaining three classifications succeeded. Initial evidence retained in `real-jev-initial.json` and `.log`. The cause of that initial transport failure is not established and no network fix is claimed.

Second run: **4/4 successful**:

| Task | Jev recommendation | Execution | Classifier latency |
|---|---|---|---:|
| Small pure helper | gpt-5.6-luna / low | gpt-5.6-luna / low | 404 ms |
| Deadlock, leases, fencing, side effects | gpt-6.1-sol / high | gpt-6.1-sol / high | 408 ms |
| Documentation sentence | gpt-5.6-luna / none | gpt-6.1-sol / low (hysteresis) | 526 ms |
| Repeated documentation stage | gpt-5.6-luna / none | gpt-5.6-luna / none | 411 ms |

Customer charge: $0.0057 each. Reported classifier input/output tokens: 1977/222, 2099/219, 2164/222, 2221/222. Stored classifier cost is the configured internal estimate based on returned usage, **not a provider invoice**. Evidence: `artifacts/gpt-auto-phase22/real-jev.json`.

## Failures found during test development

- The no-upstream fixture initially used ordinary account PUT with `schedulable`; that request DTO does not accept this field. The fixture was corrected to `POST /admin/accounts/:id/schedulable` and now asserts saved state before testing. Failed fixture evidence is retained as `e2e-fixture-failure.log` / `results-fixture-failure.json`.
- Chrome harness initially encountered the first-login onboarding overlay and incorrectly assumed the save toast had `role=status`; the existing component uses an aria-live container instead. The harness now marks onboarding complete and asserts the actual toast text plus API/save/reload state. There were no JavaScript page errors in those failed passes.
- Python's default macOS bytecode-cache path was outside the writable sandbox; script syntax was checked with `ast.parse` instead. This did not affect runtime tests.

## Guarantees and practical limits

- Global live and dynamic routing remain opt-in. Group permission revocation is checked before affinity/Jev, and underlying permissions remain enforced.
- Missing hard state, incompatible model or mismatched required account fails safely; no invented context or dropped reference. Full Redis outage returns 409 for the warm authenticated hard-continuation test with zero Jev calls.
- No verified conversation-state migration exists. `conversation` references with unverified ownership are rejected. This feature intentionally does not claim account-only cross-model transfer.
- Hard-affinity Jev diagnostics are not enabled; skipped calls are labelled rather than fabricated.
- Exact classification caching and adaptive strategy remain unavailable. The server rejects enabling them.
- Timeline is bounded to ten successful causal predecessors. Concurrent branches may share sequence numbers; only one CAS winner advances soft history. This is not a globally serialized event timeline.
- Network Jev overhead and cost intentionally grow with per-turn classification. Local synthetic latency must not be generalized to production.
- No broader balance reservation system was added. Existing actual-usage settlement and pricing snapshots remain unchanged.
- No claim that the earlier production-only 502 was fixed. Deliberately injected upstream failures retain their documented upstream error behavior; classifier/state failures use structured errors.

Rollout/rollback procedure and architecture: `GPT_AUTO_PHASE22_DYNAMIC_ROUTING_IMPLEMENTATION.md`.

## Final local acceptance results

Final working-tree source manifest: `2cc4b8c17bb94d81618f54ddf184bfd68b5f936ca781f7da5705d2457472e467`. Production Docker build and startup succeeded.

- Dynamic routing Docker acceptance: **12/12** scenarios passed (`e2e-final.log`, `results.json`).
- Phase 2.1 pricing/authorization/streaming/export regression: **19/19** passed (`phase21-regression.log`). Historical row count and charge sum remained unchanged.
- No eligible upstream: **503**; unsupported Sol `none` effort normalized to **low**.
- Redis outage with hard state: **409**, zero Jev calls; service recovered after Redis restart.
- Deliberate Jev timeout/429/auth/malformed/breaker scenarios use compatible fallback or explicit 503. Deliberate upstream 500 retains the existing explained upstream 502 response; this is not an unexplained routing error.
- Eight concurrent requests in one logical session each classify and have distinct, single settled usage records.

### Identical synthetic workloads: 100 turns per mode

| Metric | Legacy affinity-first | Dynamic per-request |
|---|---:|---:|
| Jev calls | 1 | 100 |
| Estimated classifier cost | $0.0000084 | $0.0008400 |
| Mean request latency | 6.11 ms | 49.98 ms |
| P95 request latency | 7.08 ms | 51.60 ms |
| Mean classification overhead per turn | 0.01 ms | 40.97 ms |
| Classification overhead P50 / P95 / P99 | 0 / 0 / 0.01 ms | 41 / 42 / 43 ms |
| Customer settled total | $0.5700000000 | $0.5700000000 |
| Recorded procurement total | $0.4640000000 | $0.5057600000 |
| Upgrades / downgrades | 0 / 0 | 33 / 33 |
| Distribution: 5.6 Luna / 6 Luna / 6.1 Sol | 0 / 100 / 0 | 16 / 50 / 34 |
| Routing failures in this benchmark | 0 | 0 |

Latency quantiles include skipped-classification turns as zero in legacy mode. Redis `INFO commandstats` deltas are saved in `results.json`: GET 805 → 908, EVAL 0 → 101, EVALSHA 404 → 600. These measure the whole isolated gateway workload, including budget/quota/session operations and background jobs, not just affinity.

Procurement amounts use the local configured fixture rates and actual mock usage. They are ledger observations for this synthetic workload, not external invoices or production forecasts. Dynamic switching increased classifier overhead and procurement in this particular workload; no efficiency or quality improvement is claimed.

Evidence directory: `artifacts/gpt-auto-phase22/` (ignored local artifacts, not committed). It contains build/start logs, backend/frontend/race/type/lint outputs, source manifest, benchmark output, CSV/JSONL, mock scenario results and both real-provider runs.


### Browser and final state

Headless Chrome acceptance passed: server validation displayed, dynamic enable/save/reload/revoke persisted, global live stayed OFF, English/Chinese labels rendered, mobile viewport had no horizontal overflow, dark controls rendered correctly, and a real persisted dynamic decision displayed previous/Jev/policy/executed values plus its bounded five-predecessor timeline. No page JavaScript errors. Screenshots were visually inspected after disabling capture-time animations:

- `artifacts/gpt-auto-phase22/dynamic-desktop.png`
- `artifacts/gpt-auto-phase22/dynamic-mobile-dark.png` (390 × 844)
- `artifacts/gpt-auto-phase22/dynamic-detail.png`
- `artifacts/gpt-auto-phase22/browser-results.json`

Final read-only SQL verification (`final-switches.txt`):

```text
live=false
dynamic=false
internal_only=true
classifier_url=https://mock:8443
synthetic_credential_restored=true
enabled_groups=0
```

The SQL reads only these fields plus the boolean credential-restoration comparison; no secret value is printed. Changed-source credential-literal scanning passed. Backend selected suites pass on all three packages; final service/repository race tests pass. `git diff --check` passes. All feature changes remain uncommitted on `main`.

There is no remaining blocker in the tested local acceptance scope. Production rollout still requires explicit authorization and review of the documented conservative state/account constraints, mixed-version CAS rollout rule and higher per-turn classifier overhead. The first real-provider transport failure remains unexplained; its structured error handling and subsequent successful repetition are documented rather than presented as a network fix.
