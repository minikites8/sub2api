# GPT Auto Phase 2.2: dynamic per-turn routing

## Scope and rollout

Dynamic routing is opt-in inside the existing versioned GPT Auto settings document (`live.dynamic_routing`). It defaults OFF. Neither global live routing nor group access is enabled by this change. The active pool remains exactly `gpt-5.6-luna`, `gpt-6-luna`, `gpt-6.1-sol`. Direct-model routing is unchanged. No commit, push or deployment is part of this implementation.

## Root cause and new pipeline

The previous router looked up `api_key_id + previous_response_id/session` before Jev and returned the cached model **and effort** on a hit. Successful requests refreshed its one-hour default TTL. A logical session identifier could therefore suppress classification for the entire workflow.

When dynamic routing is enabled:

1. Refresh group admission and check global/group/key/rollout permissions. Capture the existing immutable tariff snapshot.
2. Build the existing state-v3 snapshot and model/capability exclusions.
3. Determine state dependency from the actual protocol, independently of the session identifier.
4. For self-contained Chat/Responses requests, retain prior successful routing metadata as soft affinity; **classify every inference request** using state-v3.
5. Apply existing Jev/client effort policy, candidate eligibility and margin policy, then the explicit switching policy. Recalculate projected economics for the selected model.
6. Select an eligible upstream. Only pre-execution scheduling failure can reroute; it reuses the existing classification and tariff.
7. Forward once; keep actual partial usage settlement through the existing ledger. Dynamic routes do not perform handler-level account failover after an inference failure whose side effects cannot be ruled out.
8. On success, save bounded session metadata using optimistic Redis CAS and record the final executed effort after group caps.

The original affinity-first router remains available when `dynamic_routing.enabled=false`. Disable the feature to roll back switching behavior, without altering tariffs or historical usage.

## State continuity

| Request | Dependency | Behavior with dynamic routing |
|---|---|---|
| Chat containing history | none / soft | Jev each request; models and effort may change |
| Independent Responses input | none / soft | Same as Chat |
| Explicit session header or `prompt_cache_key` | soft | Correlation only; never proves upstream state ownership |
| Responses `previous_response_id` | hard_model_and_account | Require that exact authenticated-key-scoped response binding, keep model/account, skip Jev |
| Responses `conversation` without a verified binding | unknown upstream state | Fail closed with structured 409 |
| Missing/expired/unreadable response binding | unavailable | Structured `gpt_auto_state_continuation_unavailable`, no Jev |

There is no verified cross-model state transfer or full-history reconstruction facility in this gateway. We therefore retain both model and account for server-held Responses state. The scheduler's chosen account is checked before forwarding; a different account is rejected with 409, rather than being used as a substitute. This is deliberately conservative: it may reject when a required account is alive but the scheduler chooses another. Native conversation-ID binding/migration is not implemented. A raw session identifier cannot stand in for a missing response binding. No reference is removed; no tool action is replayed by this feature.

Existing Redis entries remain readable. Old response entries containing a model/account may serve as hard bindings. Old session entries are only soft metadata in dynamic mode. Dynamic completion writes a **new** response reference, never overwrites the parent reference. Hard continuations do not diagnostically invoke Jev, so they never claim a blocked Jev recommendation that was not actually obtained.

## Switching configuration

Defaults (feature itself disabled):

```json
{
  "enabled": false,
  "stateless_strategy": "per_request",
  "stateful_strategy": "preserve_required_affinity",
  "allow_model_upgrade": true,
  "allow_model_downgrade": true,
  "allow_effort_change": true,
  "downgrade_stability_count": 2,
  "enable_classification_cache": false,
  "preserve_account_affinity_when_required": true,
  "capability_tiers": {
    "gpt-5.6-luna": 1,
    "gpt-6-luna": 2,
    "gpt-6.1-sol": 3
  }
}
```

Capability tiers are explicit administrator assertions (1–100), not inferred from model names or price. Equal tiers allow lateral task-fit changes. All three pool entries must be present. The UI exposes these tiers. Existing task-fit/capability exclusions always win over retaining a prior model. No hardcoded confidence threshold is added.

Eligible upgrades are immediate when enabled. A downgrade requires 1–20 (default 2) consecutive recommendations for the **same lower model**, with the same task anchor. A different lower candidate resets the streak. Effort is evaluated independently even while a model downgrade is held. `honor_client` retains its precedence. The normal group effort cap still runs before execution and is reflected in routing headers/logs.

The anchor is a server-keyed HMAC of state-v3's current user instruction or resolved instruction behind a short follow-up. No prompt content is persisted. A changed or missing anchor, or a changed captured routing-policy snapshot, resets the streak. This intentionally avoids semantic guesses: paraphrasing or compaction can reset a streak. It may delay a legitimate downgrade, but cannot bypass a justified upgrade. Large prompt size, tool availability and code vocabulary introduce no new Sol preference.

Classifier failure still follows the explicitly configured compatible-fallback or reject policy and does not accumulate a downgrade vote. Hard bindings never enter that fallback path.

Equivalent-state classification caching and adaptive classification are **not implemented**. Server validation rejects enabling them rather than advertising an ineffective optimization. Mandatory account affinity cannot be disabled. Within one inference attempt, retries/pre-execution reroutes reuse the immutable route rather than reclassifying.

## Concurrency, storage and privacy

No database migration or new ledger is needed. Configuration uses the existing settings/audit pipeline. Telemetry is additive under the existing decision `features.switching` JSON. Old rows keep missing metadata; no historical values are reconstructed.

Redis records gain sequence, downgrade streak, task HMAC, and at most ten successful prior turns (model, effort, direction, sequence only). Atomic `CompareAndSet` updates a soft session only if its observed sequence is unchanged. Concurrent branches may share a sequence number; only one wins session persistence. Thus two overlapping responses cannot accumulate two downgrade votes or let an older branch overwrite a newer one. These sequence values describe observed causal branches, not a globally serialized conversation order. No distributed inference lock is held.

The existing Redis keys stay API-key scoped for compatibility. Newly persisted task/session correlation values use domain-separated HMAC-SHA256 with the server-held Jev credential; rotating that credential changes correlation/anchor fingerprints and resets relevant continuity metadata. Raw prompts, raw session secrets and state references are not exported. Deploy all nodes before enabling dynamic routing; an old binary cannot participate in the new CAS protocol and may overwrite a soft entry, resetting metadata. Keep compatibility mode during mixed-version rollout.

Redis outages do not make a hard state binding optional. A soft affinity-store failure can reclassify if the gateway's other shared dependencies permit it. The gateway also uses Redis for authentication, budgets and quotas, so a full Redis outage may be rejected earlier.

## Billing and economics

`requested_model=gpt-auto`, `pricing_model=gpt-auto` in fixed mode, and `executed_model=<selected pool model>` remain distinct. Existing tariff snapshots, cached-token handling, group/user factors, legacy pricing, actual-usage settlement, idempotency and cancellation behavior remain the source of truth. Jev is internal overhead, not a customer surcharge. Procurement uses executed model and upstream account. No prices are changed by dynamic routing.

For the test fixture $2 input / $0.50 cached input / $8 output per million, 1000 total input (200 cached) + 500 output costs $0.0057 across all three models. Procurement and fully loaded margin can differ. Final-model projections are recomputed if hysteresis changes the policy choice; they remain estimates, never guaranteed realized margins.

## Administration and export

Live Routing gains EN/ZH dynamic controls with existing Vue components, responsive grid and dark styling. Logs use `mode` for the live/shadow title and explanation. Live detail shows previous, raw Jev, switch-policy selection and executed model/effort, suppression reason, state dependency, classification trigger and a bounded recent successful-session timeline. Hard hits explicitly say “Not classified — stateful continuation”. Legacy hits are labelled separately.

Admin exports append `previous_model`, `previous_effort`, `policy_model`, `policy_effort`, `model_switched`, `effort_changed`, `switch_direction`, `switch_reason`, `switch_block_reason`, `classification_trigger`, `classification_cache_hit`, `affinity_type`, `state_dependency`, `session_correlation_id`, `session_decision_sequence`. Existing Jev/final model, effort, affinity outcome and finance columns are retained. Rows carrying new telemetry identify `gpt-auto-export-v3`; historical rows retain their v2 identity with null new cells. Customer export schema stays separate and contains no session correlation, internal switching reasons, procurement or account fields.

## Files

- `.gitignore`: retain the requested reports while excluding local test evidence and account data.

- `backend/internal/service/gpt_auto_dynamic.go`, `_test.go`: config, policy, state guard, HMAC, CAS contract, telemetry, tests.
- `gpt_auto_live.go`, `gpt_auto_live_config.go`, `gpt_auto_policy.go`, `gpt_auto_decision.go`: routing integration and existing JSON storage.
- `gpt_auto_export.go`, `_test.go`: append-only administrative fields and isolation tests.
- `backend/internal/repository/gpt_auto_affinity_cache.go`, `_test.go`: atomic Redis updates.
- `gpt_auto_shadow_repo.go`: bounded list metadata projection.
- `backend/internal/handler/gpt_auto_live_helper.go`, `openai_gateway_handler.go`, `openai_chat_completions.go`: account state enforcement, no unsafe dynamic failover, final effort synchronization.
- `frontend/src/api/admin/gptAuto.ts`, `GPTAutoLivePanel.vue`, `GPTAutoDecisionLog.vue`, EN/ZH `gptAuto.ts`, `GPTAutoView.spec.ts`: controls, telemetry and tests.
- `scripts/gpt-auto-local/verify_phase22.py`: Nginx routing, failure and performance acceptance; `verify_phase22_real.py`: opt-in synthetic real-provider checks; `browser_phase22.mjs`: Chrome UI acceptance. Phase 2.1 export fixture accepts additive v3 rows.

## Rollout and rollback

1. Review unit, frontend, browser and isolated Docker acceptance evidence. Keep all production switches unchanged.
2. Deploy only with explicit authorization. Finish the rolling deployment with dynamic routing OFF.
3. Confirm the existing independent tariff, group permissions, per-model capabilities, capability tier ordering and classifier budgets. Per-turn routing intentionally increases classifier calls and overhead.
4. Enable for the controlled GPT Auto audience using the existing global/internal/key/group controls; there is no automatic group enrollment. Observe classifications, suppression reasons, failures, actual procurement and margins.
5. Roll back by setting `live.dynamic_routing.enabled=false`. Global live OFF remains the broader kill switch. No database rollback, historical billing rewrite or tariff change is needed. Rolling back the binary requires dynamic OFF first; retained additive JSON fields are safe for old readers.

Limitations: no state migration, no adaptive/equivalent-input cache, conservative account rejection, bounded successful-branch timeline rather than a serialized all-events graph, no claim of answer-quality gains, and no claim that the earlier production-only 502 has been fixed. Existing group/settings propagation intervals remain unchanged.
