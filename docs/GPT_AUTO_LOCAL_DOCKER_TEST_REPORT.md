# GPT Auto local Docker validation — 2026-10-22

The initial acceptance run below used the current working tree, including uncommitted context diagnostics changes. That initial phase performed no commits or pushes. The subsequently authorized CI repair and merge into `minikites8/sub2api` `main` are recorded in the follow-up section. No production deployment, production credentials, database dumps or release tags are used in either phase. Raw evidence is in the ignored `artifacts/gpt-auto-local/` directory.

## Environment and isolation

- macOS 26.6 (25G72), Apple M5, ARM64, 10 CPU cores, 16 GiB host RAM.
- Docker Desktop 4.96.0; Engine/client 29.9.1; Compose v5.6.0. Docker VM: 10 CPUs, approximately 7.75 GiB RAM, linux/arm64.
- Host Go 1.27.3; production Go builder 1.27.2, matching `backend/go.mod`. Host Node 24.19.0; Docker Node 24.22.1; Docker pnpm 9.15.9; Vite 5.4.21.
- Dedicated Compose project/network: `gpt-auto-local` / `gpt-auto-local_default`.
- Dedicated named volumes: `gpt-auto-local_app_data`, `gpt-auto-local_postgres_data`, `gpt-auto-local_redis_data`.
- App image: `sub2api-gpt-auto-local:working-tree`, built with the repository production Dockerfile and embedded Vue assets.
- Dependencies: `postgres:18.1-alpine3.23`, `redis:8.4-alpine`, `nginx:1.28-alpine`, `python:3.13-alpine`. All inspected images are ARM64; no AMD64 emulation.
- Only loopback ports: application/proxy `http://127.0.0.1:18080`; direct backend `http://127.0.0.1:18081`; synthetic mock/control `http://127.0.0.1:18082`. PostgreSQL and Redis have no published ports.
- Generated development credentials are in ignored `.env.gpt-auto-local`, mode 0600. The example contains placeholders only. The test user, group, provider account and API keys are created through supported authenticated APIs.
- Existing unrelated containers, networks and volumes were inventoried and retained. Only this project's initially empty network was recreated when Docker Desktop did not publish ports on an `internal` network. Its named volumes were retained.

## Verified defects and changes

### Production frontend Docker build heap exhaustion

The original Docker build failed at `pnpm run build`, after i18n tests and during `vue-tsc -b`:

```text
Scavenge ... 2038.1 (2048.9) ... MB
FATAL ERROR: Ineffective mark-compacts near heap limit Allocation failed - JavaScript heap out of memory
ELIFECYCLE Command failed with exit code 134
```

A separate unchanged HEAD frontend in a disposable Node 24 ARM64 container also exhausted the default approximately 2 GiB heap. This is an existing build capacity problem, not evidence of a GPT Auto runtime crash or the production 502 cause.

The Dockerfile now gives only the frontend build command a configurable `FRONTEND_NODE_OPTIONS=--max-old-space-size=4096`. No runtime heap, gateway timeout or routing setting changes. The rebuilt frontend completed type checking and the production Vite build (1,427 modules; Vite approximately 50.6 seconds). Evidence: `build-before-heap-fix.log`, `baseline-node-default-heap.log`, `build.log`.

### GPT Auto tagged unit-test compilation collision

`go test -tags=unit` failed in both the original working tree and unchanged HEAD:

```text
gpt_auto_context_test.go:38:6: msg redeclared in this block
generate_session_hash_test.go:64:6: other declaration of msg
```

Renamed the GPT Auto test helper and its callers to `gptAutoContextMessage`. Existing diagnostics fixtures and assertions are preserved. The tagged GPT Auto and session-hash regression run now passes 164 top-level tests. This is a test compilation fix and cannot explain a runtime 502. Evidence: `go-unit-gpt-auto.log`, `go-unit-baseline.log`, `go-unit-gpt-auto-fixed.log`.

## Automated source checks

| Check | Executed result |
|---|---|
| Original `go test -p 4 ./...` | Three failing packages: routes, service, migrations; 26 top-level failures |
| Separate HEAD copy, same three packages | 25 top-level failures; the same persistent failure set |
| Extra allocation-count assertion in the first full run | Passed 20/20 focused repetitions on both working tree and HEAD; one intermittent full-suite failure retained in the evidence |
| GPT Auto race checks | Passed across service, admin handler, gateway handler and middleware |
| Tagged GPT Auto/session-hash unit tests after helper fix | 164 top-level tests passed |
| Complete `go test -tags=unit ./...` after helper fix | 41 failing top-level tests in 8 packages and an unrelated handler timeout after 10 minutes; all 41 failures and the timeout were reproduced on the HEAD copy with the same helper-only rename |
| Unfiltered repository integration tests | Baseline compilation blocker: `opsRepository.LookupDeletedKeyAudit` does not exist |
| GPT Auto integration with explicit test-only overlay | 6/6 passed: fresh schema, upgrade through 278/279, classifier versioning, pagination/usage, global Redis budget atomicity, diagnostics JSONB |
| Frontend `vue-tsc --noEmit` | Passed |
| Complete frontend unit suite | 26 failed, 4,236 passed, 9 skipped; 2 additional suites fail during collection |
| Same frontend suite on HEAD | Identical failed-test/failed-suite set; 4,222 passed (14 fewer new passing tests) |
| Focused GPT Auto components/view/i18n | 37/37 passed |
| Full ESLint | Zero errors, one existing unused-import warning in `SettingsView.spec.ts` |

The test-only integration overlay excludes exactly `ops_repo_lookup_deleted_key_audit_integration_test.go`; no application or repository implementation was bypassed. The complete repository integration suite is not claimed to pass.

The tagged handler timeout was `TestOpenAIResponsesExcelBPS429OnEveryAccountReturnsRateLimit`. Its baseline reproduction was deliberately capped at 90 seconds, rather than spending another ten minutes on the known unrelated hang. `tagged-unit-baseline-comparison.json` confirms all 41 failing test names were reproduced. No remaining failed tests are GPT Auto tests. The allocation-count assertion remains recorded as intermittent; focused repetitions passing do not make the first full-suite run green.

Source comparison evidence: `initial-git-status.txt`, `initial-working-tree.patch`, `initial-untracked.txt`, `baseline-comparison.json`. Baseline checkout: separate `/tmp/gpt-auto-local-baseline-20261022` archive of HEAD `7e1a78597144ef2129413b9c2203eeec3e93b5f4`; it was never used for the application deployment.

## Extraction performance

Measured using `GOMAXPROCS=4 go test -p 4 ./internal/service -run '^$' -bench BenchmarkGPTAutoSnapshot -benchmem -count=3` on this Mac:

| Synthetic input | Relatively idle results | Allocation |
|---|---|---|
| 11.4 MB / 10,000 agent items | 29.41, 29.69, 27.84 ms/op | approximately 1.005 MB; 5,732 allocations |
| 6,000 chat messages | 10.79, 10.66, 10.76 ms/op | approximately 1.507 MB; 9,292 allocations |

An earlier run concurrent with broad regression tests measured 133–204 ms for the agent input and 32–45 ms for chat. These are host measurements with different contention, not a production SLA or the network request latency. Evidence: `context-benchmark.log`, `context-benchmark-idle.log`.

The real Responses gateway also accepted an 11,447,926-byte, 10,001-item request (one user instruction plus 10,000 agent events). Three calls per mode measured median **522.48 ms shadow off / 552.66 ms shadow on**. The approximately 30.18 ms difference is an end-to-end observation from a small sequential sample, not an isolated profiler measurement. The stored decision confirmed all 10,001 items scanned, 5,000 tool calls and 5,000 tool results recognized, eight agent events retained, 1,165 state characters and `state-v3` diagnostics. Evidence: `large-agent-http.json`.

For small gateway requests, 40 calls per mode measured median **4.78 ms off / 4.94 ms on**, p95 **6.98 / 11.31 ms**. This small local sample includes proxy, scheduling and billing work; it is not a throughput benchmark. The final queue drained to length 0, bytes 0 and active calls 0; the circuit was closed. Configured active workers: 2; classifier concurrency limit: 4; queue capacity: 4,096; queued snapshot byte cap: 64 MiB. Queue saturation, byte bounds and cancellation/leak behavior also passed the existing GPT Auto tests.

After the large-body checks, a container snapshot measured backend 244 MiB / 3.05% CPU, PostgreSQL 57 MiB, Redis 13 MiB, mock 15 MiB and Nginx 12 MiB. Earlier backend samples were approximately 97 MiB. The body checks temporarily increase retained Go heap; this short run does not establish a long-term memory-growth bound. Runtime metric samples were approximately 185–256 goroutines across startup/restarts/load, then 238 in a later sample. No blocked inference handler, race report, persist failure or undrained queue remained after recovery. No prolonged soak test was performed. The `memory_used_mb` column in ops metrics measures VM-wide memory and must not be mistaken for backend RSS.

## Rebuild, restart and inspection

From the repository root:

```bash
scripts/test-gpt-auto-local.sh build
scripts/test-gpt-auto-local.sh start
scripts/test-gpt-auto-local.sh full

docker compose --env-file .env.gpt-auto-local -p gpt-auto-local \
  -f compose.gpt-auto-local.yml restart backend web
docker compose --env-file .env.gpt-auto-local -p gpt-auto-local \
  -f compose.gpt-auto-local.yml ps
scripts/test-gpt-auto-local.sh logs
```

`build` initializes credentials if absent, checks port availability on first initialization, records a working-tree SHA256 and validates Compose before building. `start` waits for PostgreSQL, Redis and mock health, then backend and proxy health. `full` runs authenticated HTTP/API, diagnostics, inference, streaming, error injection, restart, privacy, billing and latency checks; it always collects container logs even on a failed assertion. No command deletes volumes or performs global cleanup.

On fresh application volumes, the admin must complete the application's one-time compliance acknowledgement before admin API tests can run. This session obtained explicit user authorization limited to this local instance and used `POST /api/v1/admin/compliance/accept`; no authorization check was removed or bypassed. The script does not silently accept an agreement on a future fresh instance. Local login: `admin@gpt-auto.local`; read its generated password from `.env.gpt-auto-local`.

The HTTPS classifier mock uses a generated 30-day development CA trusted only in the backend container via `SSL_CERT_FILE`. No host trust store or TLS verification was disabled. The normal classifier client rejects private resolved IPs; testing therefore uses its supported classifier proxy setting and a local CONNECT proxy that permits exactly `mock:8443`. This fixture does not test direct public Jev DNS, certificates, account credentials or availability. Certificate files and private keys stay under ignored `artifacts/gpt-auto-local/tls/`, outside the Docker image build context. Regenerate expired local certificates with `init` after removing only these owned fixture certificates, then recreate backend/mock; the application volumes must be retained.

## Deployment verification

The corrected production image built successfully and the complete five-service stack reached health in **12.85 seconds** on the initial app start. All 381 migration records were present, including 278 and 279. PostgreSQL accepted queries, Redis returned PONG, and the backend connected to both. The backend started without panic, dependency-injection failure or SQL mismatch. Both direct and proxied `/health`, `/login` and all six initial JS/CSS asset links returned 200. The app's `env=production` log field means the release-mode local binary, not a production deployment.

The running binary reported:

```text
Sub2API gpt-auto-local
commit: 4c508e7604b1fc833eab0de155c8ba4e52ea99606c07fc183f512547e243e2fa
built: 2026-10-22T14:02:59Z
```

This commit field is the generated **working-tree SHA256**, not Git HEAD. The source hash includes uncommitted and untracked application changes. Image ID/digest: `sha256:517e2e9888b13c88016187492517867f80789455fa7821025146207890e60372`, ARM64, approximately 202 MB. `/meta` and stored decisions independently confirmed the newly compiled implementation:

| Runtime field | Observed |
|---|---|
| `state_version` | `state-v3` |
| `classifier_schema` | `jev-questions-v2` |
| `diagnostics_version` | `context-diagnostics-v1` |
| phase / live routing | `shadow` / `false` |
| Effective classifier hash | `16f4ee5c6fc0361aaa95929633587c226dd7a25c96e11add80d60a80da7bae72` |

The hash is available from the classifier/Test Lab/debugger views and stored decisions; it is not asserted to be a `/meta` field. All four requested models appeared in `/meta`. Evidence: `runtime-meta.json`, `running-version.txt`, `application-image.txt`, `migrations.txt`, `stored-decision.json`, `containers.log`.

## HTTP/API and compatibility results

**15 scenario groups executed: 14 passed, 1 failed acceptance requirement.** The final full run executed 14 groups; the additional large-agent group was then run independently and consolidated into `http-results.json`. The script now includes that group in future full runs.

- Anonymous/invalid admin tokens returned 401 through both proxy and direct backend; a real regular user received 403 for admin routes. Login used the setup-created admin and the standard login endpoint.
- Verified GET meta/config/overview/decision list/detail, PUT config and model profiles, GET/PUT classifier settings and POST classifier preview, Test Lab and State Debugger. Invalid sample rate and invalid debugger input returned 400. Classifier secrets were masked. Classifier instruction edits were versioned and restored.
- Thirteen real debugger fixtures covered simple chat, routine code, engineering, English and Chinese follow-ups, tool calls/failures, 600 and 1,202 messages, long agents, missing `previous_response_id`, images, secret redaction and truncation. Item/budget accounting invariants and diagnostics size under 4 KB passed. A debugger request over 4 MiB returned 400. The exact effective Jev state came from the existing debugger, with no alternate parser.
- Chat Completions and Responses passed through proxy and direct backend, with streaming/non-streaming and tools, including streamed function-call arguments on both endpoint families. Thirty concurrent requests per shadow mode completed. The final targeted gateway recheck matched twelve response/event comparisons after normalizing only generated creation timestamps; IDs, ordered SSE events, deltas, tool arguments, finish state and usage remained compared. Raw responses are retained in `gateway-responses.json`; the recheck summary is in `gateway-final-console.log`.
- The mock observed unchanged customer model `gpt-6.1-sol` and reasoning effort `high`, including the gateway's existing added reasoning summary field. Persisted usage compared the same account, model, effort, 10 input/5 output tokens, rate multiplier 1 and actual cost **USD 0.000070** in both shadow modes. Shadow recommendations never selected the customer model or changed billing.
- A 17 MiB request completed with 200 and incremented `skipped_too_large`; it was not classified. Over 32 MiB returned 413: Nginx HTML via the proxy, gateway JSON directly. This local test limit is intentionally below the production default.
- The persisted decision contained the current context/schema hash and bounded diagnostics, with neither the unique synthetic prompt marker nor synthetic email in its stored JSON. Debugger audit-body omission, redaction and no raw-prompt storage were also covered by unit/integration tests.

Browser validation was **attempted but not completed**. Native Chrome automation timed out/reset and did not retain a reliable dedicated localhost window; Safari also timed out, and an independent in-app browser was unavailable. No visual walkthrough or screenshot pass is claimed. HTTP asset delivery, frontend component tests and production compilation did pass.

## 502 investigation — verified observations

**The reported production-only/unexplained GPT Auto 502 was not reproduced under healthy local dependencies. Its production root cause remains unestablished.** No production logs, proxy configuration, provider credentials, existing databases or customer data were used. The following distinct local failures were actually reproduced:

| Exact endpoint / trigger | HTTP result and evidence | Verified origin |
|---|---|---|
| `POST /v1/chat/completions`, mock AI provider returns 500 or 502 | Both direct and proxied request return 502 with `{"error":{"message":"Upstream service temporarily unavailable","type":"upstream_error"}}`; logs record the upstream status and failover | Application translating an external AI provider failure; `backend/internal/handler/openai_gateway_handler.go:4328`, called by Chat Completions failover handling |
| Same endpoint, mock provider waits 4 seconds | Both paths return the same JSON 502 after approximately 2,014–2,016 ms, matching the deliberately configured 2-second local upstream-header deadline | AI upstream timeout; backend remains alive and admin APIs remain usable |
| `GET /health`, only the dedicated backend stopped | Nginx returned HTML 504 to the final explicit request; health probes also recorded 502, with `connect() failed (111: Connection refused)` / upstream resolution or timeout logs. Direct backend connection was refused | Genuine reverse-proxy upstream unavailability under intentional backend stop, not Jev or request parsing |
| `POST /api/v1/admin/gpt-auto/test`, initial direct private mock configuration | JSON 502 `GPT_AUTO_CLASSIFIER_FAILED`, `Jev classification failed: transport`; direct private-IP resolution was rejected by the production HTTP client guard | Expected Test Lab classifier transport failure; `newGPTAutoHTTPClientFactory` enables `ValidateResolvedIP` in `gpt_auto_shadow_service.go:1576`. The fixture was corrected to use verified TLS and the supported explicit proxy; Test Lab then returned 200 |

The original image build OOM happened before any application container could be deployed and is **not** established as the production 502 cause. No gateway/runtime code was changed to disguise an upstream failure or make a 502 into a success. `upstream-errors.json`, proxy error lines in `containers.log`, `http-fixture-debugging.log` and `http-before-compliance.log` retain the before/after evidence. Initial harness assertions were tightened to check actual Jev error classes and corrected to allow normal per-request timestamps and real model-cooldown recovery; failed interim fixture runs are retained rather than reported as successful acceptance runs.

## Failure injection and remaining availability failure

| Scenario | Observed result |
|---|---|
| Jev delayed 2 seconds, classifier timeout 500 ms | Customer request 200 in approximately 10.7 ms; stored shadow error `timeout` |
| Jev 401 and 403 | Customer 200; shadow error `auth` |
| Malformed JSON / incomplete answers | Customer 200; shadow error `invalid_answer` |
| Five classifier failures | Circuit opened; customer request still 200; after normal 30-second cooldown, half-open probe recovered and circuit closed |
| Dedicated PostgreSQL restart | `pg_isready`, admin config, migrations and inference recovered without recreating the backend |
| Dedicated backend stop/start | Proxy unavailable during stop, direct connection refused; startup, background services, current `/meta` and inference recovered |
| Dedicated Redis stop, shadow on | Customer **503 after 8,337 ms**, health endpoint still 200; shadow `skipped_budget_unavailable` incremented by 1 |
| Dedicated Redis stop, shadow off | Same customer **503 after 8,337 ms**, health endpoint still 200; no shadow-budget increment |
| Redis restored | PONG, admin APIs and inference recovered without data loss; all five services healthy at handoff |

The remaining failed acceptance requirement is **customer inference continuing while the whole Redis service is unavailable**. It fails with shadow both enabled and disabled. Backend logs identify `openai.user_slot_acquire_failed`; `AcquireUserSlot` returns the Redis acquisition error (`backend/internal/service/concurrency_service.go:424`), and `openai_gateway_handler.go:2513` handles that existing concurrency-admission failure. This is distinct from GPT Auto budget reservation, which correctly fails closed and skips classification.

The gateway's `/health` is a liveness check and remained 200 during this dependency failure; it must not be treated as proof of inference readiness when Redis is unavailable. Compose also reports the separate Redis service health. Changing concurrency admission to silently fail open would change the gateway's existing capacity/security semantics, so that unrelated policy was not changed merely to make this test green. The strict continuity check deliberately keeps `scripts/test-gpt-auto-local.sh full` at exit status **1** and records the failure in `http-results.json`. Normal operation and the shadow-isolation checks pass.

## Commands executed

The principal commands below were actually run; their stdout/stderr are retained under the corresponding evidence filenames:

```bash
docker version
docker compose version
scripts/test-gpt-auto-local.sh init
scripts/test-gpt-auto-local.sh build
scripts/test-gpt-auto-local.sh start
scripts/test-gpt-auto-local.sh full

cd backend
GOMAXPROCS=4 go test -p 4 ./...
GOMAXPROCS=4 go test -p 4 -tags=unit ./...
GOMAXPROCS=4 go test -p 4 -tags=unit ./internal/service \
  ./internal/handler/admin ./internal/handler ./internal/server/middleware \
  ./cmd/server -run 'GPTAuto|GenerateSessionHash' -count=1 -v
GOMAXPROCS=4 go test -p 4 -race ./internal/service \
  ./internal/handler/admin ./internal/handler ./internal/server/middleware \
  -run GPTAuto -count=1
GOMAXPROCS=4 go test -p 4 -tags=integration ./internal/repository -run GPTAuto -count=1 -v
GOMAXPROCS=4 go test -p 4 -overlay ../artifacts/gpt-auto-local/integration-overlay.json \
  -tags=integration ./internal/repository -run GPTAuto -count=1 -v
GOMAXPROCS=4 go test -p 4 ./internal/service -run '^$' \
  -bench BenchmarkGPTAutoSnapshot -benchmem -count=3
```

Frontend checks used the installed dependency executables with the host Node runtime: `node node_modules/vue-tsc/bin/vue-tsc.js --noEmit`, `node node_modules/vitest/vitest.mjs run`, the focused GPT Auto/i18n Vitest run and `node node_modules/eslint/bin/eslint.js . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts`. The host pnpm launcher reported v11 and could not find its own `pnpm install` subprocess, so it was not used for dependency resolution; no lockfile was changed. The actual production Docker build used frozen pnpm 9.15.9 and performed `check:i18n`, `vue-tsc -b`, `vite build` and the embedded Go build.

## Final recommendation

**NO-GO against the full requested availability acceptance criteria.** The current-source isolated stack builds, starts and serves GPT Auto in shadow mode, and no GPT Auto-induced normal-traffic 502 was found. Two verified build/test defects are fixed. However, whole-Redis-outage customer continuity remains unsatisfied, and the specific production 502 remains unverified. Production rollout should not be represented as fully validated by this local run.

At the initial acceptance checkpoint, remaining limitations were: the existing Redis dependency and liveness/readiness distinction; repository-wide baseline failures and timeout (subsequently repaired below); the unfiltered integration compilation blocker (subsequently repaired below); no completed visual browser walkthrough; no real public TypeSafe/provider credential/DNS/TLS test; no access to the actual production reverse proxy, load balancer, resource limits or logs; no long soak or multi-node production load test. The local mock validates gateway contracts and classifier failure handling, not the quality of real Jev recommendations.

The healthy local stack is left running at `http://127.0.0.1:18080/admin/gpt-auto` for manual inspection. Only synthetic local data and development credentials are present. No unrelated Docker resources were removed. A final comparison confirmed that all originally dirty tracked files, except the explicitly appended local-test ignore rules, still match the initial working-tree patch; new changes are the Docker build fix, test-helper namespace fix and the requested local validation assets.


## Follow-up: CI remediation (2026-10-22–23)

The user subsequently authorized commits/pushes and requested CI/CD repair. The user explicitly requested merging both the prior GPT Auto work and these
fixes into `minikites8/sub2api` `main`. Validation is performed on
`fix/gpt-auto-local-ci-20261022` before a fast-forward merge and push. A `main`
push normally publishes GHCR images through Package; no production deployment
is performed.

Verified upstream failures at source commit `7e1a78597144ef2129413b9c2203eeec3e93b5f4`:

- CI run `39743178478`: frontend (6 assertions), unit-test compilation (GPT Auto
  helper collision), further baseline test failures, and 49 lint findings.
- Security Scan run `39743178401`: reachable standard-library vulnerability
  GO-2026-6809 in Go 1.27.2. Module, Docker builders and workflow checks now use
  Go 1.27.3; local govulncheck reports zero reachable vulnerabilities.
- Package run `39743178473` passed before these repairs.

Changes preserve the enabled lint/security checks. Fixes cover SQL pricing
argument order and omitted fields, deep copying of time pricing, group/account
validation, public-settings DTO omissions, signed-thinking SSE preservation,
media sticky concurrency waits, and BPS request compatibility. Test fixtures are
updated for explicit group BPS opt-in, paid versus gift balance, retired platform
CHECK constraints, current retry semantics, and transaction visibility. The
obsolete deleted-credential lookup test is removed because that API was retired;
existing deletion tests still require that raw deleted credentials are not retained.

Frontend CI now includes the GPT Auto view and diagnostics component. All 83
critical test files / 1422 tests pass locally; typecheck and ESLint are also run.
One existing unused-import ESLint warning remains, with zero errors.

The initial observations above are retained as historical evidence. Final full
backend/remote workflow results are recorded below after verification. The whole
Redis outage continuity limitation and unverified production 502 remain separate
from CI repair; this follow-up does not authorize a production rollout.

Additional failures uncovered after the original compilation blocker were fixed
without disabling checks: explicit paid-subscription renewal fixtures, SQL-only
gift-balance schema fields in SQLite fixtures, current temporary-disable fields,
Kiro's restored platform registration, and SSE tests that waited for staged
metadata before sending content. Production corrections include canonical GPT-6
pricing lookup, enforcing the already-computed channel restriction at sticky
admission, reporting channel restriction when no eligible account remains,
retaining only bounded Retry-After metadata through the BPS bridge, respecting
the proxy pool during disabled-account recovery probes, and preventing the
generic OAuth 502 retry policy from overriding synthesized image-text failover.
The image test still verifies that a plain-text answer does not cool the account.

The 16 GiB host reached approximately 12.7 GiB swap use with concurrent full
compilation, lint and frontend build. Those validation attempts were stopped and
checks were rerun with GOMAXPROCS=2 / -p 2 and reduced concurrency. One interim
unit run also had a stale generated test entry after a test-name edit during
compilation; it is not reported as a successful run. A Redis Testcontainers
startup deadline under that load is retained in ci-integration-round2.log.

Final local unit run: `GOMAXPROCS=2 go test -p 2 -tags=unit -timeout=10m ./...`
completed successfully (all packages; service package 247.498 s). Evidence:
`ci-unit-complete.log`. The subsequent lint-only type assertion corrections
are also included in the final integration/lint verification below.

Final local integration run: `GOMAXPROCS=2 go test -p 2 -tags=integration
-timeout=10m ./...` completed successfully without an overlay (all packages;
repository 49.746 s, service 172.136 s). Evidence: ci-integration-complete.log.
The previous unfiltered integration blocker and repository-wide unit failures
listed in the initial acceptance section are now resolved.

Final lint run: golangci-lint v2.14.0 with `--concurrency=2 --timeout=30m
--max-same-issues=0 --max-issues-per-linter=0` completed with **0 issues**.
Default result truncation had hidden further unchecked type assertions; all
were corrected, with no check exclusions. Related service and route tests pass
in ci-type-assert-regression.log (service 2.825 s, routes 4.069 s).

### Final Docker rebuild and HTTP verification (2026-10-23)

The final production Dockerfile build succeeded, including i18n checks, vue-tsc,
Vite (frontend stage 76.4 s), and the embedded ARM64 Go backend. Both Dockerfiles
now use Go 1.27.3 and the configurable 4096 MiB frontend build heap. The rebuilt
local image is `sub2api-gpt-auto-local:working-tree`, ID
`sha256:b7ab8d91e15f1a81c2ec3732f24f2b1d221b197471375947478977f97ce16fd4`,
ARM64, 208177594 bytes. Running binary source manifest:
`8a246bbeb2a8cb4cc635fdc9ff70ef2a7d55feae74a688357b1d47806d3073ea`.

`scripts/test-gpt-auto-local.sh start` completed successfully; five services
were healthy and 381 migrations were present. The full HTTP/failure-injection
run again passed **14 of 15** groups; the only failure remains
`redis_outage_customer_continuity`. It is deliberately not suppressed by the
harness. Jev delay/401/403/malformed/incomplete cases all left customer requests
at 200, approximately 6–8 ms. PostgreSQL/backend restarts recovered, current
runtime versions were confirmed, persisted billing/account/model/effort were
identical with shadow off and on, and stored decisions contained no raw prompt.

The 11,447,926-byte / 10,001-item agent request passed in both modes. End-to-end
median of three calls was 521.35 ms off / 517.66 ms on; this includes HTTP,
parsing, scheduling and billing and does not isolate snapshot extraction. Small
request medians were 6.58 ms off / 5.33 ms on (40 calls each). These short,
sequential samples are noisy and do not imply a speedup. The queue drained to
zero bytes/items/active calls and the circuit closed. Evidence:
ci-docker-build-final.log, ci-docker-start-final.log, ci-docker-full-final.log,
http-results.json, latency.json and containers.log (ignored local artifacts).

The initial repository-wide CI blockers are repaired. **The strict production
recommendation remains NO-GO for the originally requested whole-Redis-outage
continuity criterion.** The production-only unexplained GPT Auto 502 remains
unreproduced and its cause unestablished. Merging code is authorized separately
from production deployment. No production deployment or release tag is made.
The test stack remains available at http://127.0.0.1:18080/admin/gpt-auto.
