# GPT Auto Phase 2.1 acceptance report

Date: 2026-10-24. Branch `main`, uncommitted working tree. Implementation details, migration, deployment and rollback instructions: [implementation report](GPT_AUTO_INDEPENDENT_PRICING_IMPLEMENTATION_REPORT.md).

## Result

The local reverse-proxy suite passed **19/19 scenarios**. Real Chrome UI acceptance passed with **no page errors**. Targeted Go regressions passed in service, handler, admin handler, repository and middleware packages. Database migration/export/auth/CAS and balance/subscription deduplication integration tests passed. GPT Auto race tests passed. Final frontend targeted tests passed **49/49** (an earlier broader run passed 57/57), type checking and changed-component lint passed. The Docker image builds the Go backend and embedded Vue frontend; the existing Nginx `web` service serves/proxies that application. PostgreSQL, Redis and the combined mock Jev/upstream fixture run in the isolated `gpt-auto-local` stack.

No production service, production routing flag, commit, push or deployment was changed. Test scripts restore the **local** global live switch and synthetic group switch OFF. No claim is made about fixing the original production-only 502.

After the final image rerun, all five local services were healthy. Database verification returned `live_enabled=false`, `internal_only=true`, pricing version `40`, and **zero enabled groups**. Evidence: `final-stack-status.txt` and `final-switch-status.txt` under `artifacts/gpt-auto-phase21/`.

## Observed financial results

The fixture tariff was $2 uncached input, $0.50 cached input and $8 output per million tokens. Each request reported 1,000 total input (200 cached) and 500 output, including reasoning. The ledger recorded 800 uncached input, 200 cache reads, 500 output and multiplier 1.

| Executed model | Customer charge | Executed-model fixture cost | Gross profit before Jev |
|---|---:|---:|---:|
| gpt-5.6-luna | $0.0057000000 | $0.0023200000 | $0.0033800000 |
| gpt-6-luna | $0.0057000000 | $0.0046400000 | $0.0010600000 |
| gpt-6.1-sol | $0.0057000000 | $0.0069600000 | −$0.0012600000 |

These are synthetic test prices, not production recommendations. Admin exports matched final charges/account costs and their computed profits; negative margins were retained. A separate balance observation verified exactly one `$0.0057` deduction for one settled request.

- Group factor 0.8: `$0.00456`.
- Exact group override $1/$0.25/$4 with factor 0.8: `$0.00228`.
- Effective group preview: $0.8/$0.2/$3.2 per million, marked `group_override`.
- A stream admitted before an output-rate change to $80/M retained its old version and `$0.0057` charge.
- A terminal failed stream reporting partial token usage settled `$0.0057` through the normal pipeline.
- Legacy test retained the executed-model charge ×1.5 and a single balance deduction.
- Final settlement regression verified Free Fast charges `$0.0057` for priority/fast and preserves the Flex charge of `$0.00285`.
- A before/after aggregate over pre-run usage IDs verified historical charge count and sum unchanged.

## Reverse-proxy scenarios

All requests used `http://127.0.0.1:18080`, through Nginx. SQL was used for read-only assertions; supported admin APIs created/updated synthetic fixtures.

1. Startup assets/migrations, supported authentication and synthetic customer setup.
2. Database default false and newly created group default false.
3. Disabled group: absent from models, Chat and Responses return `404 model_not_found`, zero Jev/upstream calls.
4. Saved enabled switch, internal-only key restriction, configured group rollout restriction (including listed-key bypass attempt), empty rollout behavior.
5. Missing/negative/non-finite/out-of-range/overprecision tariff rejection, confirmation, stale-version conflict and non-admin denial.
6. Jev selects each of the three active models; identical customer charge, different executed-model costs, exact balance deduction and persisted pricing metadata.
7. Group factor and explicit model price override applied once, effective preview verified.
8. Pricing edit during SSE and metered failed-stream settlement.
9. Unauthorized recommended underlying model rerouted to the permitted candidate; missing virtual entitlement denied before classification.
10. Group revocation beats stored affinity immediately; ordinary direct model still available.
11. Two distinct HTTP requests with the same client header receive distinct generated request IDs and exactly one usage row/deduction each. Hosted unpriced tool rejected before Jev. This gateway does **not** offer client-header HTTP replay deduplication; existing internal settlement deduplication is tested separately against PostgreSQL.
12. Legacy billing compatibility.
13. Chat/Responses, SSE, function tool calling and tool-result follow-up.
14. Session and previous-response affinity.
15. Classifier timeout, 429, auth error, malformed/incomplete answers, configured fallback/reject policy, upstream failure, no-compatible-model failure.
16. Client cancellation.
17. Sixty concurrent requests, all successful.
18. Admin CSV/JSONL, filtering/count, final billing joins, v2 pricing metadata, customer isolation, redaction and export permissions/audit.
19. Historical charges unchanged; safe local switch cleanup.

Detailed scenario outputs and exact per-request IDs are in `artifacts/gpt-auto-phase21/phase21-results.json`; summary is `phase21-summary.json`. Export fixtures remain local in the same directory and are ignored by Git.

## UI and database acceptance

Chrome exercised the actual built Vue application: price editing, `$0.0057` preview, disabled save before confirmation, confirmed save/reload, group creation default OFF, enable/save/edit/reload/revoke, global-off and internal-only warnings, mobile width, dark rendering and Chinese localization. Screenshots: `pricing-desktop.png`, `pricing-mobile-dark.png`, `pricing-mobile-zh.png`. The mobile table intentionally scrolls horizontally within its card; the document does not overflow the viewport.

Disposable PostgreSQL tests applied the complete existing Phase 2 schema, inserted an existing group/model allowlist and historical decision, then applied migration 282 twice. Existing groups stayed OFF, the allowlist and historical decimal value survived, and repeat migration was a no-op. Additional tests verified group auth projection, old/new JSON pricing representations, concurrent settings CAS (one winner), decision round trips, export joins, and ledger deduplication for balance, subscription and archived keys.

## Executed commands and evidence

Commands below are relative to repository root unless a working directory is stated. Routine reads (`rg`, `sed`, `git status`, `git diff`) inspected the actual implementation before editing. Baseline branch/status/diff are saved in `artifacts/gpt-auto-phase21/baseline-status.txt` and `baseline.patch`.

```sh
git status --short
git branch --show-current
git diff --check
bash scripts/test-gpt-auto-local.sh build > artifacts/gpt-auto-phase21/build-final.log 2>&1
docker compose --env-file .env.gpt-auto-local -p gpt-auto-local -f compose.gpt-auto-local.yml up -d --wait --wait-timeout 300 backend web
docker compose --env-file .env.gpt-auto-local -p gpt-auto-local -f compose.gpt-auto-local.yml restart mock
python3 scripts/gpt-auto-local/verify_phase21.py > artifacts/gpt-auto-phase21/e2e.log 2>&1
PATH=/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH node scripts/gpt-auto-local/browser_phase21.mjs > artifacts/gpt-auto-phase21/browser.log 2>&1
docker compose --env-file .env.gpt-auto-local -p gpt-auto-local -f compose.gpt-auto-local.yml ps > artifacts/gpt-auto-phase21/final-stack-status.txt
docker compose --env-file .env.gpt-auto-local -p gpt-auto-local -f compose.gpt-auto-local.yml exec -T postgres psql -U gpt_auto_local -d gpt_auto_local -At -c "SELECT value::jsonb->'live'->>'live_enabled' AS live_enabled, value::jsonb->'live'->>'internal_only' AS internal_only, value::jsonb->'pricing'->>'version' AS pricing_version FROM settings WHERE key='gpt_auto_shadow_config'; SELECT count(*) AS enabled_groups FROM groups WHERE gpt_auto_enabled;" > artifacts/gpt-auto-phase21/final-switch-status.txt
```

From `backend/`:

```sh
go test ./internal/service ./internal/handler ./internal/handler/admin ./internal/repository ./internal/server/middleware -run 'TestGPTAuto|TestGroupPricingJSON|Test.*Models.*|Test.*ProfitControl|TestOpenAIGatewayServiceRecordUsage|Test.*Group.*|Test.*Channel.*' -count=1 > ../artifacts/gpt-auto-phase21/backend-final.log 2>&1
CI=true go test -tags=integration ./internal/repository -run 'TestUsageBillingRepositoryApply_Deduplicates|TestGPTAuto' -count=1 -v > ../artifacts/gpt-auto-phase21/integration-tests.log 2>&1
go test -race ./internal/service -run 'TestGPTAuto' -count=1 > ../artifacts/gpt-auto-phase21/race-tests.log 2>&1
go test ./internal/service -run 'TestGroupPricingJSONLegacyAndPublicAPI' -count=1 > ../artifacts/gpt-auto-phase21/json-pricing-final.log 2>&1
go test ./internal/service -run 'TestGPTAutoFreeFast|TestGPTAutoFixedTariffActualSettlement|TestGroupPricingJSON' -count=1 > ../artifacts/gpt-auto-phase21/final-billing-check.log 2>&1
```

Frontend checks:

```sh
PATH=/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH node frontend/node_modules/vue-tsc/bin/vue-tsc.js --noEmit -p frontend/tsconfig.json > artifacts/gpt-auto-phase21/frontend-types.log 2>&1
PATH=/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH node frontend/node_modules/vitest/vitest.mjs run --root frontend src/views/admin/__tests__/GPTAutoPricing.spec.ts src/views/admin/__tests__/GPTAutoView.spec.ts src/views/admin/__tests__/groupModelAllowlist.spec.ts src/components/user/__tests__/GPTAutoRoutingHistory.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts > artifacts/gpt-auto-phase21/frontend-final.log 2>&1
PATH=/Users/user/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH node frontend/node_modules/eslint/bin/eslint.js frontend/src/components/admin/gptAuto/GPTAutoPricingPanel.vue frontend/src/components/admin/gptAuto/GPTAutoGroupControl.vue frontend/src/views/admin/__tests__/GPTAutoPricing.spec.ts > artifacts/gpt-auto-phase21/frontend-lint.log 2>&1
```

The Docker build additionally executes the existing locale completeness check, `vue-tsc -b`, Vite production build and embedded Go server build. The final source manifest is `artifacts/gpt-auto-local/source-manifest.sha256`: `3663e6433675b7b59ed5952c5a20050c40506e5c9b63cac8983ad3de9db882b6`.

## Failures found and resolved

Early runs intentionally did not pass. Fixes were verified by rerunning the affected checks:

- Auth projection omitted the new switch; added field and bumped auth snapshot version.
- Listed API keys could bypass configured group rollout; tightened the admission rule.
- Group factor resolution retained a cached old default; virtual snapshot now resolves the existing user override against the fresh group factor.
- Group pricing API silently ignored snake_case rates; accepts public names and historical persisted names, including cache creation.
- Free Fast initially flattened the Flex discount; final settlement regression verifies that only priority/fast premiums are removed.
- Metered terminal failed passthrough result was dropped; preserved for GPT Auto settlement while failover results remain unbilled.
- Initial catalogue filtering changed ordinary ETag behavior; restricted transformation to virtual-model catalogues and reran ordinary-model regressions.
- Vue saved-status watcher cleared success text; fixed watcher ordering. Prop mutation lint failures replaced with update events.
- Migration fixtures accidentally included future migration 282 in pre-Phase-1 schemas; corrected fixture composition.
- A test assumed client request-header replay semantics that the gateway does not have; corrected it and ran real repository deduplication tests.
- Browser assertions initially read asynchronous warnings too early; now wait for rendered state. Browser/API configuration mutation tests run serially.
- A Docker rebuild encountered an Alpine package-fetch failure; subsequent build succeeded. Sandbox-local HTTP/cache restrictions required approved reruns. Existing jsdom navigation and bundle-size/Browserslist warnings did not fail tests.

## Limits and release decision

This is local synthetic acceptance, not a production canary. Full repository-wide tests were not run; the targeted regression set covers changed billing, group/channel configuration, permissions, model catalogue, export and UI areas. No real paid upstream or Jev call was made. Real procurement depends on configured account/channel costs; projections cannot guarantee profitability. Existing balance/quota prechecks are not a reservation system. Unknown usage cannot be billed accurately without upstream data. Global tariff/config propagation across backend nodes retains the 30-second cache TTL; fresh persisted group switch revocations do not wait for that cache. Unsupported separately priced media/hosted tools are rejected in independent mode.

Release remains an operator decision. Follow the implementation report's backup, additive migration, explicit tariff confirmation and per-group enrollment procedure. Roll back only after disabling global live routing and draining in-flight requests, because the preceding Phase 2 binary cannot enforce the new group switch. Preserve historical usage and additive columns.
