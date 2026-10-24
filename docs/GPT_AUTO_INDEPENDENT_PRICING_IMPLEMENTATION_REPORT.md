# GPT Auto Phase 2.1 implementation

Date: 2026-10-24. Branch: `main`. This implementation builds on the substantial uncommitted Phase 2 and partial Phase 2.1 work already present. No commit, push or deployment is authorized or performed. Only the isolated `gpt-auto-local` test configuration is changed during acceptance; its global live switch and synthetic group switch are restored OFF.

## Investigation and architecture

The original Phase 2 settlement runs the selected model through `OpenAIGatewayService.RecordUsage`, then scales `CostBreakdown.ActualCost` by the GPT Auto profile multiplier. `TotalCost`, account cost and usage buckets continue to describe the executed model. Settlement flows through the existing usage billing command/repository; request ID and API key identity provide idempotency. There is no full pre-execution monetary reservation system for these text requests. Balance/quota eligibility remains the existing gateway check; this change does not promise prevention of concurrent overspend.

Phase 2.1 retains three identities:

| Identity | Independent mode | Purpose |
|---|---|---|
| Requested | `gpt-auto` | Customer request, usage history |
| Pricing | `gpt-auto` | Customer tariff and applicable customer rules |
| Executed | One of the three pool models | Scheduling, capability checks, upstream usage, procurement |

The active pool remains exactly `gpt-5.6-luna`, `gpt-6-luna`, `gpt-6.1-sol`. Retired models remain readable in historical records and usable as ordinary gateway models where existing configuration permits them.

The versioned tariff is stored in the existing GPT Auto settings document. The existing channel pricing system primarily supplies execution/channel prices; changing that to represent the virtual retail identity would contaminate procurement accounting. Therefore the GPT Auto administration page contains a dedicated retail entry, linked from channel pricing and group forms, while group overrides reuse `Group.ModelPricing` and group/user factors reuse the existing mechanisms. No new financial ledger is introduced.

## Configuration and migration

`282_gpt_auto_independent_pricing.sql` adds:

- `groups.gpt_auto_enabled BOOLEAN NOT NULL DEFAULT FALSE`, for existing and new groups.
- Additive decision columns: pricing model, mode, version, source and four decimal tariff rates.

No historical usage or charge is rewritten. Historical decision pricing fields remain empty/null. Existing persisted GPT Auto settings without a pricing section read as `legacy_executed_model_multiplier`, including installations whose live switch is temporarily off. A missing settings document defaults to `virtual_model_fixed_rates` with an incomplete tariff, which cannot admit traffic. An administrator must explicitly configure and confirm a tariff. Neither migration nor pricing save enables global live routing or enrolls any group.

Independent mode requires explicit input, cached-input and output rates, in USD per million tokens. Zero is an explicit choice; null is unconfigured. Decimal parsing rejects NaN/infinity, negative values, rates above $1,000/M and precision beyond six decimal places. An optional cache-creation rate defaults to the input rate. Exact `gpt-auto` group overrides replace supplied fields; wildcard real-model price entries do not silently replace the virtual tariff. Invalid effective override rates fail admission. The existing group JSON interface was repaired to accept the Vue snake_case fields while continuing to read historically persisted Go field names.

Pricing updates require confirmation and the expected version. The settings repository uses compare-and-swap on the full document, preventing concurrent pricing/general-configuration saves from overwriting a reviewed tariff or reusing a version. Update time/actor are recorded and existing admin audit middleware records pricing and group mutations.

## Calculation and immutable snapshot

At admission, the route captures the global tariff version, effective group override, virtual-model user/group factor, peak factor, model-billing policy and Free Fast state. Group and user factors are resolved for `gpt-auto`, independently of Jev's selection. The account customer factor remains the existing selected-account policy and is applied once at settlement. Thus account-specific customer terms may legitimately differ; changing only the executed model with identical applicable customer terms does not change the charge.

Upstream OpenAI input includes cache reads and separately reported cache creation. Existing settlement first splits it into exclusive buckets. The virtual tariff consumes those buckets; it does not subtract cached input a second time. Output already includes reasoning tokens and is billed once. Calculation uses decimal arithmetic, then rounds to the ledger's ten decimal places before passing through its existing float interfaces.

Fixture example: total input 1,000, cache reads 200, output 500, rates $2/$0.50/$8 per million:

`800×2/1M + 200×0.5/1M + 500×8/1M = $0.0057`.

With group factor 0.8 the charge is `$0.00456`. With exact group rates $1/$0.25/$4 and factor 0.8 it is `$0.00228`. Legacy mode still uses the executed model's price and profile multiplier; fixed mode never compounds that multiplier. Existing service-tier semantics apply to the virtual price (priority/fast ×2, flex ×0.5). Free Fast removes only the priority/fast premium; it preserves the Flex discount. Underlying model reasoning/context price premiums are not virtual-model retail rates.

The route survives request-body rewriting, upstream retries, model fallback and detached usage-recording work. Midstream tariff edits affect subsequent routes, not the in-flight route. Metered partial GPT Auto passthrough failures now retain their result for the same usage settlement path; retryable pre-output failovers do not create an extra settlement result. Unknown/unreported usage remains unknown rather than fabricated.

Supported tariff dimensions are text/vision input token quantities, cached input, separately reported cache creation and output tokens. Client function/custom tools remain supported. Hosted tools with separate charges, image generation, audio and video are rejected before Jev in fixed mode. Unexpected upstream media usage is logged and retains ordinary nonzero pricing rather than being silently free; operators must keep upstream capability behavior consistent with the admitted request.

## Permissions and visibility

All admission conditions remain mandatory: global live enabled, persisted group switch enabled, authorized selected group, virtual-model entitlement, API key authentication, rollout restrictions, and an eligible underlying profile with required permissions/capabilities. Internal-only mode requires the explicit key allowlist. Outside internal-only mode, a configured group rollout list restricts every key, including keys in the internal list; empty means all explicitly enabled groups.

The group ID comes from existing authenticated key/group selection. No client-provided group identity is trusted. Ordered multi-group keys check the switch and underlying model capacity without attempting to schedule `gpt-auto` upstream. Production `Route` and `/v1/models` perform a fresh group read, fail closed on lookup failure, and recheck model permissions before affinity/classification. A completed admin switch revocation therefore applies to subsequent requests without waiting for the auth-cache TTL. Existing auth invalidation remains in place; missing legacy cache fields decode false. Pricing/global configuration on other nodes retains the existing 30-second cache propagation interval. Already admitted in-flight requests complete under their snapshot.

Default and provider/pinned catalogues filter any upstream-supplied `gpt-auto` identity before conditionally adding the authorized virtual entry. Ordinary catalogue ETag behavior is preserved when virtual visibility does not modify the catalogue. Chat Completions, Responses and their existing aliases share the same routing helper. Denied switch/rollout requests return generic `404 model_not_found`, invoke no Jev and incur no classifier overhead. Existing middleware may reject an explicit model allowlist violation earlier with its normal forbidden response.

## Economics, administration and export

Expected revenue uses the captured virtual tariff; candidate procurement uses configured channel assumptions or the existing executed-model pricing lookup. Jev overhead is included in fully loaded projections. Unknown output/price assumptions remain insufficient/unknown; losses retain their sign. The existing explicitly configured GPT Auto margin guard may adjust a recommendation; independent pricing alone does not authorize that adjustment. The optional account profit gate uses a conservative ratio of virtual rates to executed-model list rates instead of an unrelated executed-model customer discount. These gates are estimates, not a realised-profit guarantee.

Realised financial reporting remains sourced from `usage_logs.actual_cost` and the existing account procurement-cost fields. Gross profit = charge − account cost; fully loaded profit additionally subtracts recorded Jev cost. Model profile procurement estimates do not replace settled account costs.

The Vue administration includes a pricing tab, active mode/status/version/update time, four rate fields, group-adjusted/override preview, comparison across the three execution models, token calculator, negative/unknown margin warning and explicit confirmation after preview. Editing the draft invalidates confirmation. Group create/edit forms include the existing Toggle component, default OFF, coherent model-selector enrollment, conflict repair control, pricing link, global-off notice and internal-key restriction notice. English/Chinese strings and responsive/dark styles are included. Live decision detail shows pricing identity/mode/version.

Exports use append-only `gpt-auto-export-v2`: original v1 columns remain in order and pricing fields are appended. Old records are exported from stored usage, never repriced. Customer export excludes account procurement costs, margin, Jev/internal diagnostics and credentials.

## Main file areas

- `backend/internal/service/gpt_auto_pricing*.go`, `gpt_auto_live*.go`, `gpt_auto_config.go`, `gpt_auto_economics.go`: tariff, snapshot, admission, simulation and economics.
- `openai_gateway_usage.go`, `openai_gateway_passthrough.go`, `openai_profit_control.go`: existing settlement and optional profit gate integration.
- `group.go`, `admin_group.go`, `admin_service.go`, auth snapshot files, group/API-key repositories, Ent group schema/generated files: persistent group switch.
- `api_key_groups.go`, handler live helper, model catalogue handlers, Chat/Responses handlers: authenticated selection, visibility and request propagation.
- `setting_repo.go`, `channel.go`, `channel_pricing_json.go`: concurrent configuration safety and compatible existing group pricing serialization.
- Admin pricing handler/routes/audit registration, decision repository/export schema, migration 282.
- Vue pricing/group components, group forms/types, GPT Auto view/API/locales, channel-pricing link and decision detail.
- Pricing, billing, frontend and database integration tests; `scripts/gpt-auto-local/verify_phase21.py`, browser harness and extended mock fixtures.

The acceptance report records commands, outcomes and remaining test limitations. The initial working-tree inventory and diff are preserved under `artifacts/gpt-auto-phase21/` to distinguish pre-existing changes.

## Deployment and rollback procedure (not executed)

1. Back up database and GPT Auto settings; retain the preceding application image. Review production procurement prices and intended customer terms.
2. Deploy the additive migration with the application using the project's normal migration startup. Existing/new groups remain OFF. No automated global activation is included.
3. Review legacy mode preservation, configure a tariff in the pricing page, select representative groups, examine projections and confirm the versioned save.
4. With separate operational authorization, enable only intended groups and rollout keys. Validate real usage and margins before wider enrollment. Keep the three-model pool unchanged.
5. To disable service, revoke group switches or turn global live routing OFF. To restore old pricing for new requests, explicitly confirm legacy mode. In-flight requests retain their tariff.
6. For application rollback, first turn global live routing OFF and drain in-flight requests. Older Phase 2 binaries do not enforce the new group switch; never roll back to them while live routing is enabled. Keep additive columns and historical records; no destructive down migration or financial rewrite is required.

Known boundaries: no monetary reservation, estimates depend on operator prices/history, cross-node tariff rollout can take 30 seconds, account-specific customer terms remain applicable, separately priced hosted/media capabilities are unavailable under the token tariff, and local synthetic tests cannot establish the cause or resolution of the original production-only 502.
