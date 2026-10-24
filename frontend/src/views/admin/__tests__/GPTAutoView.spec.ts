import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GPTAutoView from '../GPTAutoView.vue'
import GPTAutoConfigPanel from '@/components/admin/gptAuto/GPTAutoConfigPanel.vue'
import GPTAutoDecisionLog from '@/components/admin/gptAuto/GPTAutoDecisionLog.vue'
import GPTAutoTestLab from '@/components/admin/gptAuto/GPTAutoTestLab.vue'
import GPTAutoClassifierPanel from '@/components/admin/gptAuto/GPTAutoClassifierPanel.vue'
import GPTAutoDecisionDetail from '@/components/admin/gptAuto/GPTAutoDecisionDetail.vue'
import GPTAutoStateDebugger from '@/components/admin/gptAuto/GPTAutoStateDebugger.vue'
import GPTAutoLivePanel from '@/components/admin/gptAuto/GPTAutoLivePanel.vue'
import type { GPTAutoConfig, GPTAutoDecision, GPTAutoOverview, GPTAutoPolicyDecision } from '@/api/admin/gptAuto'

const mocks = vi.hoisted(() => ({
  api: {
    getMeta: vi.fn(),
    getConfig: vi.fn(),
    updateConfig: vi.fn(),
    getOverview: vi.fn(),
    listDecisions: vi.fn(),
    getDecision: vi.fn(),
    replayDecision: vi.fn(),
    runTest: vi.fn(),
    simulate: vi.fn(),
    getClassifier: vi.fn(),
    updateClassifier: vi.fn(),
    previewClassifier: vi.fn(),
    compareClassifier: vi.fn(),
    debugState: vi.fn(),
    countExport: vi.fn(),
    exportDecisions: vi.fn(),
  },
  showSuccess: vi.fn(),
  showError: vi.fn(),
  replace: vi.fn(),
}))

vi.mock('@/api/admin/gptAuto', () => ({
  gptAutoAPI: mocks.api,
  default: mocks.api,
  gptAutoInstructionFields: ['model_selection', 'effort_selection', 'complexity_scoring', 'category_classification'],
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }) }))
vi.mock('@/utils/apiError', () => ({ extractApiErrorMessage: (_err: unknown, fallback: string) => fallback }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('vue-chartjs', () => ({ Bar: { template: '<div data-testid="bar" />' }, Line: { template: '<div data-testid="line" />' } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, te: () => false }),
}))

const stubs = {
  Icon: true,
  SmartOpsNav: true,
  Pagination: true,
  Toggle: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<button type="button" class="toggle" @click="$emit(\'update:modelValue\', !modelValue)">{{ modelValue }}</button>',
  },
  ConfirmDialog: {
    props: ['show'],
    emits: ['confirm', 'cancel'],
    template: '<div v-if="show" data-testid="confirm-dialog"><button data-testid="confirm-yes" @click="$emit(\'confirm\')" /></div>',
  },
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  Select: {
    props: ['modelValue', 'options'],
    emits: ['update:modelValue', 'change'],
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\', $event.target.value)"><option v-for="o in options" :key="String(o.value)" :value="o.value">{{ o.label }}</option></select>',
  },
}

const profile = (model: string, efforts = ['none', 'low', 'medium', 'high', 'xhigh', 'max']) => ({
  model,
  description: `${model} description`,
  enabled: true,
  billing_multiplier: 1.2,
  routing_weight: 60,
  supported_efforts: efforts,
  supports_tools: true,
  upstream_costs: [],
})

const baseConfig = (): GPTAutoConfig => ({
  shadow_enabled: false,
  sample_rate: 0,
  endpoints: ['chat_completions', 'responses'],
  all_groups: true,
  group_ids: [],
  model_filter: [],
  classifier_base_url: 'https://api.typesafe.ai',
  classifier_model: 'jev-latest',
  classifier_timeout_ms: 8000,
  classifier_max_concurrency: 4,
  classifier_retry_count: 1,
  classifier_daily_call_limit: 10000,
  classifier_daily_cost_limit: 5,
  classification_max_context_chars: 4000,
  classification_context_messages: 4,
  classification_context_strategy: 'task_aware',
  classification_agent_steps: 8,
  worker_count: 2,
  queue_size: 4096,
  decision_retention_days: 30,
  global_min_margin: 0.3,
  profit_safety_buffer: 0.05,
  model_profiles: [profile('gpt-5.6-luna'), profile('gpt-6-luna'), profile('gpt-6.1-sol', ['low', 'medium', 'high', 'xhigh', 'max'])],
  retired_model_profiles: [profile('gpt-5.6-sol')],
  model_pool_version: 'pool-v2',
  live: liveConfig(),
  policy_version: 'auto-v2',
  classifier_api_key_configured: true,
  classifier_api_key_masked: 'ts-k****1234',
})

function liveConfig() {
  return {
    dynamic_routing: { enabled: false, stateless_strategy: 'per_request', stateful_strategy: 'preserve_required_affinity', allow_model_upgrade: true, allow_model_downgrade: true, allow_effort_change: true, downgrade_stability_count: 2, enable_classification_cache: false, preserve_account_affinity_when_required: true, capability_tiers: { 'gpt-5.6-luna': 1, 'gpt-6-luna': 2, 'gpt-6.1-sol': 3 } },
    live_enabled: false,
    internal_only: true,
    allowed_groups: [],
    allowed_api_keys: [12],
    endpoints: ['chat_completions', 'responses'],
    classifier_timeout_ms: 2500,
    classifier_max_concurrency: 16,
    classifier_retry_count: 0,
    classifier_failure_policy: 'compatible_fallback',
    fallback_model: 'gpt-6-luna',
    fallback_effort: 'medium',
    client_effort_policy: 'jev',
    session_affinity_enabled: true,
    session_affinity_ttl_seconds: 3600,
    margin_guard_enabled: false,
    apply_billing_multiplier: true,
    live_request_budget: 10000,
    customer_export_enabled: true,
  } as const
}

const meta = {
  models: baseConfig().model_profiles.map((p) => ({ model: p.model, native_supported_efforts: p.supported_efforts })),
  efforts: ['none', 'low', 'medium', 'high', 'xhigh', 'max'],
  categories: [],
  endpoints: ['chat_completions', 'responses'],
  default_profiles: baseConfig().model_profiles,
  policy_version: 'shadow-v1',
  phase: 'shadow',
  live_routing_active: false,
}

const overview = (): GPTAutoOverview => ({
  shadow_enabled: true,
  sample_rate: 5,
  classifier_configured: true,
  policy_version: 'shadow-v1',
  start_time: '2026-10-20T00:00:00Z',
  end_time: '2026-10-21T00:00:00Z',
  error_rate: 0.1,
  stored: {
    total: 10, success: 9, failed: 1, avg_latency_ms: 420, input_tokens: 3000, output_tokens: 40,
    classifier_cost: 0.000126, adjusted: 2, no_eligible: 0, below_target_count: 1,
    final_models: [{ key: 'gpt-6-luna', count: 9 }], jev_models: [{ key: 'gpt-6-luna', count: 9 }],
    final_efforts: [{ key: 'medium', count: 9 }], complexity: [{ key: '2', count: 9 }],
    categories: [{ key: 'coding_debugging', count: 9 }], error_classes: [{ key: 'timeout', count: 1 }],
    actual_vs_final: [{ actual: 'gpt-5.6-sol', recommended: 'gpt-6-luna', count: 9 }],
    trend: [{ bucket: '2026-10-20T10:00:00Z', success: 9, failed: 1 }],
    trend_bucket_size: 'hour',
  },
  runtime: {
    counters: { observed: 100, sampled: 10, dropped_queue_full: 0 },
    queue_length: 0, queue_capacity: 4096, queue_bytes: 0, active_calls: 0, circuit_state: 'closed',
    today_calls: 10, today_cost: 0.000126, budget_scope: 'cluster', cluster_counters_today: { sampled: 42, skipped_budget: 5, dropped_queue_full: 3 },
    recent_latency_samples: 9, excluded_endpoints: ['messages'],
  },
})

const policyDecision = (finalModel = 'gpt-6.1-sol', finalEffort = 'low'): GPTAutoPolicyDecision => ({
  policy_version: 'shadow-v1',
  status: 'adjusted',
  jev_model: 'gpt-6.1-sol',
  jev_effort: 'none',
  final_model: finalModel,
  final_effort: finalEffort,
  candidates: [{ model: 'gpt-6.1-sol', eligible: true, probability: 0.7, routing_weight: 90 }],
  adjustments: [{ rule: 'effort_unsupported', from: 'none', to: 'low' }],
  capability_warnings: ['effort_unsupported'],
})

const classification = {
  classifier_model: 'jev-1.13.0', model: 'gpt-6.1-sol', model_confidence: 0.7, model_probabilities: { 'gpt-6.1-sol': 0.7 },
  effort: 'none', effort_confidence: 0.5, complexity: 2.4, complexity_confidence: 0.6, category: 'coding_debugging',
  category_confidence: 0.8, input_tokens: 300, output_tokens: 4,
}

const decision = (): GPTAutoDecision => ({
  id: 7, created_at: '2026-10-20T10:00:00Z', request_id: 'req-7', endpoint: 'chat_completions',
  actual_model: 'gpt-5.6-sol', actual_effort: 'high', stream: true, has_tools: false,
  evaluation_status: 'success', decision_status: 'adjusted', jev_model: 'gpt-6.1-sol', jev_effort: 'none',
  final_model: 'gpt-6.1-sol', final_effort: 'low', complexity: 2.4, category: 'coding_debugging',
  model_confidence: 0.7, classifier_model: 'jev-1.13.0', classifier_input_tokens: 300, classifier_output_tokens: 4,
  classifier_cost: 0.0000126, classifier_latency_ms: 420, queue_delay_ms: 3, attempts: 1, margin_below_target: false,
  policy_version: 'shadow-v1', policy_snapshot_hash: 'abcdef123456',
  features: { endpoint: 'chat_completions', has_tools: false, estimated_input_tokens: 100 },
  classification, decision: policyDecision(),
  actual_usage: {
    model: 'gpt-5.6-sol', requested_model: 'gpt-5.6-sol', upstream_model: '', reasoning_effort: 'high',
    input_tokens: 1200, output_tokens: 800, cache_read_tokens: 0, list_cost: 0.01, charged_cost: 0.015, account_cost: 0.006,
    created_at: '2026-10-20T10:00:05Z',
  },
})

const mountWith = (component: unknown) => mount(component as never, { global: { stubs } })

let wrapper: ReturnType<typeof mount> | undefined

beforeEach(() => {
  vi.clearAllMocks()
  mocks.api.getMeta.mockResolvedValue(meta)
  mocks.api.getConfig.mockResolvedValue(baseConfig())
  mocks.api.updateConfig.mockImplementation(async (payload: GPTAutoConfig) => ({ ...baseConfig(), ...payload }))
  mocks.api.getOverview.mockResolvedValue(overview())
  mocks.api.listDecisions.mockResolvedValue({ items: [decision()], total: 1, page: 1, page_size: 20, pages: 1 })
  mocks.api.getDecision.mockResolvedValue(decision())
  mocks.api.replayDecision.mockResolvedValue({ decision_id: 7, mode: 'current', original: policyDecision(), replayed: policyDecision('gpt-6-luna', 'medium'), changed: true })
  mocks.api.runTest.mockResolvedValue({ classification, decision: policyDecision(), classifier_cost: 0.0000126, latency_ms: 410, snapshot_preview: '{"latest_user_request":"hi"}', profiles_overridden: false })
  mocks.api.getClassifier.mockResolvedValue(classifierView())
  mocks.api.updateClassifier.mockImplementation(async (payload: Record<string, string>) => ({ ...classifierView(), instructions: { ...payload, revision: 1 }, is_default: false }))
  mocks.api.previewClassifier.mockImplementation(async (payload: Record<string, string>) => ({
    hash: 'feedbeef0000', same_as_saved: false,
    effective: { schema_version: 'jev-questions-v2', state_version: 'state-v2', classifier_model: 'jev-latest', questions: { target_model: { type: 'choice', instructions: payload.model_selection, criteria: { 'gpt-6-luna': 'efficient' } } } },
  }))
})

const instructionText = (prefix: string) => ({
  model_selection: `${prefix} model selection instruction text`,
  effort_selection: `${prefix} effort selection instruction text`,
  complexity_scoring: `${prefix} complexity scoring instruction text`,
  category_classification: `${prefix} category classification instruction text`,
})

function classifierView() {
  return {
    instructions: { ...instructionText('SAVED'), revision: 3 },
    defaults: instructionText('DEFAULT'),
    is_default: false,
    hash: 'abc123def4567890',
    effective: { schema_version: 'jev-questions-v2', state_version: 'state-v2', classifier_model: 'jev-latest', questions: {} },
    history: [{ hash: 'abc123def4567890', revision: 3, created_at: '2026-10-21T10:00:00Z', created_by: 1, snapshot: { schema_version: 'jev-questions-v2', state_version: 'state-v2', classifier_model: 'jev-latest', questions: {} } }],
    limits: { min_runes: 20, max_runes: 4000 },
    legacy_instructions: {},
  }
}

afterEach(() => wrapper?.unmount())

describe('GPTAutoView', () => {
  it('shows the shadow-mode disclosure and real overview data', async () => {
    wrapper = mountWith(GPTAutoView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-phase-badge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-auto-shadow-banner"]').text()).toContain('gptAuto.shadowBanner.body')
    expect(mocks.api.getOverview).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="gpt-auto-tile-sampled"]').text()).toContain('10')
    expect(wrapper.find('[data-testid="gpt-auto-actual-vs-final"]').text()).toContain('gpt-5.6-sol')

    await wrapper.find('[data-testid="gpt-auto-tab-logs"]').trigger('click')
    await flushPromises()
    expect(mocks.api.listDecisions).toHaveBeenCalled()
    expect(mocks.replace).toHaveBeenCalledWith({ query: { tab: 'logs' } })
  })
})

describe('GPTAutoView cluster scope', () => {
  it('shows all-node counters and flags an unreachable budget store', async () => {
    wrapper = mountWith(GPTAutoView)
    await flushPromises()
    const cluster = wrapper.find('[data-testid="gpt-auto-cluster-counters"]')
    expect(cluster.exists()).toBe(true)
    expect(cluster.text()).toContain('42')
    expect(wrapper.find('[data-testid="gpt-auto-tile-skipped"]').text()).toContain('5')
    expect(wrapper.find('[data-testid="gpt-auto-budget-unavailable"]').exists()).toBe(false)
    wrapper.unmount()

    const degraded = overview()
    degraded.runtime.budget_scope = 'unavailable'
    degraded.runtime.cluster_counters_today = undefined
    mocks.api.getOverview.mockResolvedValue(degraded)
    wrapper = mountWith(GPTAutoView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-budget-unavailable"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-auto-cluster-counters"]').exists()).toBe(false)
  })
})

describe('GPTAutoConfigPanel', () => {
  it('saves independent multiplier and weight edits without echoing the API key', async () => {
    wrapper = mountWith(GPTAutoConfigPanel)
    await flushPromises()
    expect((wrapper.find('[data-testid="gpt-auto-api-key"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('[data-testid="gpt-auto-disclosure"]').exists()).toBe(true)

    await wrapper.find('[data-testid="gpt-auto-multiplier-gpt-6-luna"]').setValue('1.7')
    await wrapper.find('[data-testid="gpt-auto-weight-gpt-5.6-luna"]').setValue('300')
    await wrapper.find('[data-testid="gpt-auto-save"]').trigger('click')
    await flushPromises()

    expect(mocks.api.updateConfig).toHaveBeenCalledTimes(1)
    const payload = mocks.api.updateConfig.mock.calls[0][0]
    const byModel = Object.fromEntries(payload.model_profiles.map((p: { model: string }) => [p.model, p]))
    expect(byModel['gpt-6-luna'].billing_multiplier).toBe(1.7)
    expect(byModel['gpt-6-luna'].routing_weight).toBe(60)
    expect(byModel['gpt-5.6-luna'].routing_weight).toBe(300)
    expect(byModel['gpt-5.6-luna'].billing_multiplier).toBe(1.2)
    expect(byModel['gpt-5.6-sol']).toBeUndefined()
    expect(payload.live).toEqual(liveConfig())
    expect(payload.classifier_api_key).toBe('')
    expect(mocks.showSuccess).toHaveBeenCalled()
  })

  it('requires confirmation before enabling shadow classification', async () => {
    wrapper = mountWith(GPTAutoConfigPanel)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-shadow-toggle"]').trigger('click')
    await wrapper.find('[data-testid="gpt-auto-save"]').trigger('click')
    expect(mocks.api.updateConfig).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="confirm-dialog"]').exists()).toBe(true)
    await wrapper.find('[data-testid="confirm-yes"]').trigger('click')
    await flushPromises()
    expect(mocks.api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ shadow_enabled: true }))
  })

  it('blocks saving invalid values and supports reset', async () => {
    wrapper = mountWith(GPTAutoConfigPanel)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-multiplier-gpt-6-luna"]').setValue('-1')
    expect(wrapper.find('[data-testid="gpt-auto-save"]').attributes('disabled')).toBeDefined()
    await wrapper.find('[data-testid="gpt-auto-reset"]').trigger('click')
    expect((wrapper.find('[data-testid="gpt-auto-multiplier-gpt-6-luna"]').element as HTMLInputElement).value).toBe('1.2')
  })

  it('does not allow enabling efforts the gateway rejects', async () => {
    wrapper = mountWith(GPTAutoConfigPanel)
    await flushPromises()
    const noneBox = wrapper.find('[data-testid="gpt-auto-profile-gpt-6.1-sol"] input[type="checkbox"][value="none"]')
    expect(noneBox.attributes('disabled')).toBeDefined()
  })
})

describe('GPTAutoDecisionLog', () => {
  it('opens a decision and replays the policy without a classifier call', async () => {
    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-decision-7"]').trigger('click')
    await flushPromises()
    expect(mocks.api.getDecision).toHaveBeenCalledWith(7)
    expect(wrapper.find('[data-testid="gpt-auto-adjustments"]').text()).toContain('none')
    const actual = wrapper.find('[data-testid="gpt-auto-actual-usage"]')
    expect(actual.text()).toContain('gpt-5.6-sol')
    expect(actual.text()).toContain('$0.015')
    expect(actual.text()).toContain('gptAuto.detail.actualBadge')

    await wrapper.find('[data-testid="gpt-auto-replay-run"]').trigger('click')
    await flushPromises()
    expect(mocks.api.replayDecision).toHaveBeenCalledWith(7, { mode: 'current' })
    expect(mocks.api.runTest).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="gpt-auto-replay"]').text()).toContain('gpt-6-luna')
  })

  it('says when no usage-log entry exists for the request', async () => {
    mocks.api.getDecision.mockResolvedValue({ ...decision(), actual_usage: undefined })
    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-decision-7"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-actual-usage"]').text()).toContain('gptAuto.detail.actualMissing')
  })

  it('shows context diagnostics for a decision and the unavailable notice for older ones', async () => {
    const withDiagnostics = {
      ...decision(),
      classifier_config_hash: 'feedface00112233',
      features: {
        endpoint: 'responses', has_tools: true, estimated_input_tokens: 100,
        context: {
          state_version: 'state-v3', strategy: 'task_aware' as const, items_total: 9, scanned_bytes: 900, latest_user_index: 8, latest_user_kind: 'instruction' as const,
          latest_user_truncated: false, follow_up_anchor_index: -1, original_request_index: 0, context_turns_available: 2, context_turns_included: 2,
          agent_events_included: 0, agent_events_omitted: 0, budget_chars: 4000, used_chars: 300,
          diagnostics: {
            diagnostics_version: 'context-diagnostics-v1', scan_status: 'scanned' as const, context_truncated: false,
            selection: [{ section: 'latest_user_request' as const, status: 'included' as const, chars: 300, items: 1 }],
            follow_up: { resolution: 'not_follow_up' as const, short_message: false, original_task_available: true, agent_activity_available: false },
            completeness: 'sufficient' as const,
          },
        },
      },
    }
    mocks.api.getDecision.mockResolvedValue(withDiagnostics)
    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    expect(mocks.api.getMeta).toHaveBeenCalled()
    await wrapper.find('[data-testid="gpt-auto-decision-7"]').trigger('click')
    await flushPromises()
    const section = wrapper.find('[data-testid="gpt-auto-context-diagnostics"]')
    expect(section.exists()).toBe(true)
    expect(section.find('[data-testid="gpt-auto-diagnostics-budget"]').exists()).toBe(true)
    expect(section.find('[data-testid="gpt-auto-diagnostics-fact-classifierHash"]').text()).toContain('feedface00')
    expect(section.find('[data-testid="gpt-auto-diagnostics-comparison"]').exists()).toBe(true)
    expect(section.find('[data-testid="gpt-auto-diagnostics-unavailable"]').exists()).toBe(false)

    mocks.api.getDecision.mockResolvedValue(decision())
    await wrapper.find('[data-testid="gpt-auto-decision-7"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-unavailable"]').text()).toContain('gptAuto.diagnostics.unavailable')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-budget"]').exists()).toBe(false)
  })

  it('renders an empty state', async () => {
    mocks.api.listDecisions.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-decisions-empty"]').exists()).toBe(true)
  })
})

describe('GPTAutoTestLab', () => {
  it('analyzes a prompt through the backend and shows the policy result', async () => {
    wrapper = mountWith(GPTAutoTestLab)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-lab-prompt"]').setValue('Prove the lemma')
    await wrapper.find('[data-testid="gpt-auto-lab-run"]').trigger('click')
    await flushPromises()
    expect(mocks.api.runTest).toHaveBeenCalledWith(expect.objectContaining({ prompt: 'Prove the lemma', endpoint: 'chat_completions', model_profiles: undefined }))
    expect(wrapper.find('[data-testid="gpt-auto-final-model"]').text()).toBe('gpt-6.1-sol')
    expect(wrapper.find('[data-testid="gpt-auto-final-effort"]').text()).toBe('low')
  })

  it('shows an error when Jev is not configured', async () => {
    mocks.api.runTest.mockRejectedValue({ status: 400 })
    wrapper = mountWith(GPTAutoTestLab)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-lab-prompt"]').setValue('hello')
    await wrapper.find('[data-testid="gpt-auto-lab-run"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-lab-error"]').text()).toBe('gptAuto.lab.failed')
  })
})

describe('GPTAutoClassifierPanel', () => {
  it('edits one instruction independently and saves through its own endpoint', async () => {
    wrapper = mountWith(GPTAutoClassifierPanel)
    await flushPromises()
    expect((wrapper.find('[data-testid="gpt-auto-instr-model_selection"]').element as HTMLTextAreaElement).value).toBe('SAVED model selection instruction text')
    expect(wrapper.find('[data-testid="gpt-auto-instr-history"]').text()).toContain('abc123def456')

    await wrapper.find('[data-testid="gpt-auto-instr-effort_selection"]').setValue('EDITED effort selection instruction text')
    await wrapper.find('[data-testid="gpt-auto-instr-save"]').trigger('click')
    await flushPromises()
    expect(mocks.api.updateClassifier).toHaveBeenCalledTimes(1)
    const payload = mocks.api.updateClassifier.mock.calls[0][0]
    expect(payload).toEqual({ ...instructionText('SAVED'), effort_selection: 'EDITED effort selection instruction text' })
    expect(mocks.api.updateConfig).not.toHaveBeenCalled()
    expect(mocks.showSuccess).toHaveBeenCalled()
  })

  it('restores defaults, validates length and previews without saving', async () => {
    wrapper = mountWith(GPTAutoClassifierPanel)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-instr-default-model_selection"]').trigger('click')
    expect((wrapper.find('[data-testid="gpt-auto-instr-model_selection"]').element as HTMLTextAreaElement).value).toBe('DEFAULT model selection instruction text')

    await wrapper.find('[data-testid="gpt-auto-instr-complexity_scoring"]').setValue('too short')
    expect(wrapper.find('[data-testid="gpt-auto-instr-save"]').attributes('disabled')).toBeDefined()
    await wrapper.find('[data-testid="gpt-auto-instr-restore-all"]').trigger('click')
    expect(wrapper.find('[data-testid="gpt-auto-instr-save"]').attributes('disabled')).toBeUndefined()

    await wrapper.find('[data-testid="gpt-auto-instr-preview"]').trigger('click')
    await flushPromises()
    expect(mocks.api.previewClassifier).toHaveBeenCalledWith(instructionText('DEFAULT'))
    expect(wrapper.find('[data-testid="gpt-auto-instr-preview-result"]').text()).toContain('DEFAULT model selection')
    expect(mocks.api.updateClassifier).not.toHaveBeenCalled()
  })
})

describe('GPTAutoTestLab A/B comparison', () => {
  it('sends the draft for two Jev calls and shows both arms', async () => {
    const savedResult = { classification, decision: policyDecision(), classifier_cost: 0.00005292, classifier_input_tokens: 1260, classifier_output_tokens: 233, latency_ms: 400, snapshot_preview: '{}', profiles_overridden: false }
    const draftResult = { ...savedResult, classification: { ...classification, model: 'gpt-6-luna' }, decision: policyDecision('gpt-6-luna', 'low') }
    mocks.api.compareClassifier.mockResolvedValue({
      saved: { classifier_config_hash: 'aaaaaaaaaaaa', result: savedResult },
      draft: { classifier_config_hash: 'bbbbbbbbbbbb', result: draftResult },
      jev_calls: 2, total_classifier_cost: 0.00010584, snapshot_preview: '{}',
    })
    wrapper = mountWith(GPTAutoTestLab)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-lab-compare-toggle"]').setValue(true)
    await flushPromises()
    expect(mocks.api.getClassifier).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="gpt-auto-lab-compare-cost"]').exists()).toBe(true)
    await wrapper.find('[data-testid="gpt-auto-lab-draft-model_selection"]').setValue('DRAFT model selection instruction text')
    await wrapper.find('[data-testid="gpt-auto-lab-prompt"]').setValue('Reverse a list in Python')
    await wrapper.find('[data-testid="gpt-auto-lab-run"]').trigger('click')
    await flushPromises()

    expect(mocks.api.runTest).not.toHaveBeenCalled()
    const payload = mocks.api.compareClassifier.mock.calls[0][0]
    expect(payload.draft_instructions).toEqual({ ...instructionText('SAVED'), model_selection: 'DRAFT model selection instruction text' })
    expect(payload.prompt).toBe('Reverse a list in Python')
    const modelRow = wrapper.find('[data-testid="gpt-auto-compare-row-model"]').text()
    expect(modelRow).toContain('gpt-6.1-sol')
    expect(modelRow).toContain('gpt-6-luna')
    expect(wrapper.find('[data-testid="gpt-auto-compare-row-tokens"]').text()).toContain('1,260')
    expect(wrapper.find('[data-testid="gpt-auto-compare-row-cost"]').text()).toContain('$0.00005292')
    expect(wrapper.find('[data-testid="gpt-auto-lab-compare-summary"]').text()).toContain('gptAuto.lab.compareSummary')
    expect(mocks.api.updateClassifier).not.toHaveBeenCalled()
  })

  it('passes explicit workload assumptions to the single test', async () => {
    wrapper = mountWith(GPTAutoTestLab)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-lab-prompt"]').setValue('hi')
    await wrapper.find('[data-testid="gpt-auto-lab-assume-input_tokens"]').setValue('3000')
    await wrapper.find('[data-testid="gpt-auto-lab-assume-cached_input_tokens"]').setValue('1000')
    await wrapper.find('[data-testid="gpt-auto-lab-assume-output_tokens"]').setValue('800')
    await wrapper.find('[data-testid="gpt-auto-lab-run"]').trigger('click')
    await flushPromises()
    expect(mocks.api.runTest).toHaveBeenCalledWith(expect.objectContaining({ assumptions: { input_tokens: 3000, cached_input_tokens: 1000, output_tokens: 800 } }))
  })
})

describe('GPTAutoDecisionDetail economics', () => {
  const economics = {
    hypothetical: true, model: 'gpt-6-luna', effort: 'none', billing_multiplier: 1.1,
    estimated_input_tokens: 1, estimated_output_tokens: 10, output_estimate_source: 'admin_assumption', output_estimate_samples: 0,
    list_input_per_m: 0.09999999999999999, list_output_per_m: 0.55, upstream_input_per_m: 0.5, upstream_output_per_m: 0.45, upstream_cost_source: 'configured_channel',
    estimated_revenue: 0.000006, estimated_upstream_cost: 0.000005, estimated_classifier_cost: 0.00005292,
    estimated_gross_profit: -0.00005192, estimated_gross_margin: -8.653333, target_margin: 0.43, below_target: true, pricing_available: true,
    estimate_quality: 'admin_assumption' as const, model_gross_margin: 0.331, model_margin_status: 'below_target' as const, fully_loaded_margin_status: 'loss' as const,
  }

  it('shows model, fully loaded and overhead separately with distinct statuses', () => {
    const decision = { ...policyDecision('gpt-6-luna', 'none'), status: 'accepted' as const, jev_model: 'gpt-6-luna', jev_effort: 'none', adjustments: [], warnings: [{ rule: 'model_margin_below_target' }, { rule: 'fully_loaded_margin_loss' }], economics }
    wrapper = mountWith({ components: { GPTAutoDecisionDetail }, template: '<GPTAutoDecisionDetail :classification="c" :decision="d" />', data: () => ({ c: classification, d: decision }) })
    const model = wrapper.find('[data-testid="gpt-auto-model-margin"]')
    expect(model.text()).toContain('33.1%')
    expect(model.text()).toContain('below_target')
    expect(model.find('.text-amber-600').exists()).toBe(true)
    const full = wrapper.find('[data-testid="gpt-auto-full-margin"]')
    expect(full.text()).toContain('-865.3%')
    expect(full.text()).toContain('loss')
    expect(full.find('.text-red-600').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-auto-classifier-overhead"]').text()).toContain('$0.00005292')
    expect(wrapper.find('[data-testid="gpt-auto-economics"]').text()).not.toContain('0.0999999')
    expect(wrapper.find('[data-testid="gpt-auto-adjustments"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-auto-no-adjustments"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-auto-warnings"]').text()).toContain('model_margin_below_target')
  })

  it('shows N/A instead of a margin when data is insufficient', () => {
    const insufficient = { ...economics, estimate_quality: 'insufficient_data' as const, estimated_gross_margin: undefined, model_gross_margin: undefined, model_margin_status: 'unknown' as const, fully_loaded_margin_status: 'unknown' as const, output_estimate_source: 'insufficient_data', output_estimate_samples: 3 }
    const decision = { ...policyDecision(), economics: insufficient }
    wrapper = mountWith({ components: { GPTAutoDecisionDetail }, template: '<GPTAutoDecisionDetail :classification="c" :decision="d" />', data: () => ({ c: classification, d: decision }) })
    expect(wrapper.find('[data-testid="gpt-auto-estimate-insufficient"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-auto-model-margin"]').text()).toContain('N/A')
    expect(wrapper.find('[data-testid="gpt-auto-full-margin"]').text()).toContain('N/A')
    expect(wrapper.find('[data-testid="gpt-auto-full-margin"]').text()).not.toContain('%')
  })
})

describe('GPTAutoStateDebugger', () => {
  const debugResult = {
    options: { max_chars: 4000, context_turns: 4, strategy: 'task_aware', agent_steps: 8 },
    classifiable: true,
    jev_request: {
      model: 'jev-latest',
      state: '{"latest_user_request":"fix it","latest_user_request_kind":"follow_up","task_context":{"continues_request":"Implement an LRU cache"},"agent_activity":{"recent":[{"kind":"tool_result","status":"error","summary":"--- FAIL: TestExpiry"}]}}',
      questions: { target_model: { type: 'choice', instructions: 'pick least capable', criteria: { 'gpt-6-luna': 'efficient general model' } } },
    },
    state_chars: 70,
    estimated_jev_input_tokens: 1900,
    context: {
      state_version: 'state-v3', strategy: 'task_aware', items_total: 1203, scanned_bytes: 40000, latest_user_index: 1202, latest_user_kind: 'follow_up',
      latest_user_truncated: false, follow_up_anchor_index: 1200, original_request_index: 0, context_turns_available: 1000, context_turns_included: 4,
      agent_events_included: 3, agent_events_omitted: 0, budget_chars: 4000, used_chars: 900, section_chars: { latest_user_request: 6 },
      limitations: ['earlier_context_omitted'],
      diagnostics: {
        diagnostics_version: 'context-diagnostics-v1', scan_status: 'scanned', context_truncated: false,
        items: { eligible: 1203, represented: 7, selected_messages: 5, budget_dropped: 0, budget_truncated: 0, not_selected: 1196, privacy: 0, unsupported: 0, external_references: 0, compacted: 0, empty: 0, with_images: 0, conversation_length_known: true },
        selection: [{ section: 'latest_user_request', status: 'included', chars: 6, items: 1 }, { section: 'continues_request', status: 'included', chars: 22, items: 1 }],
        follow_up: { resolution: 'linked_request', short_message: true, original_task_available: true, agent_activity_available: true },
        completeness: 'partial', completeness_reasons: ['earlier_context_omitted'], state_chars: 70,
      },
    },
    effective: { schema_version: 'jev-questions-v2', state_version: 'state-v3', classifier_model: 'jev-latest', questions: {} },
    classifier_config_hash: 'abcdef0123456789',
    estimated_input_tokens: 9000,
  }

  it('sends the synthetic request and shows the exact state with truncation metadata', async () => {
    mocks.api.debugState.mockResolvedValue(debugResult)
    wrapper = mountWith(GPTAutoStateDebugger)
    await wrapper.find('[data-testid="gpt-auto-debug-example-longChat"]').trigger('click')
    await wrapper.find('[data-testid="gpt-auto-debug-run"]').trigger('click')
    await flushPromises()
    const payload = mocks.api.debugState.mock.calls[0][0]
    expect(payload.endpoint).toBe('chat_completions')
    expect((payload.request.messages as unknown[]).length).toBeGreaterThan(512)
    expect(payload.strategy).toBeUndefined()
    expect(wrapper.find('[data-testid="gpt-auto-debug-state"]').text()).toContain('"latest_user_request_kind": "follow_up"')
    expect(wrapper.find('[data-testid="gpt-auto-debug-info"]').text()).toContain('1,203')
    expect(wrapper.findAll('[data-testid="gpt-auto-debug-limitation"]')).toHaveLength(1)
    expect(mocks.api.runTest).not.toHaveBeenCalled()
  })

  it('shows the server-built context extraction preview without storing the fixture', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    mocks.api.debugState.mockResolvedValue(debugResult)
    wrapper = mountWith(GPTAutoStateDebugger)
    for (const example of ['simple', 'routineCode', 'complexEngineering', 'followUp', 'agentFailure', 'longChat', 'previousResponse', 'multimodal']) {
      expect(wrapper.find(`[data-testid="gpt-auto-debug-example-${example}"]`).exists()).toBe(true)
    }
    await wrapper.find('[data-testid="gpt-auto-debug-example-followUp"]').trigger('click')
    expect((wrapper.find('[data-testid="gpt-auto-debug-request"]').element as HTMLTextAreaElement).value).toContain('"continue"')
    await wrapper.find('[data-testid="gpt-auto-debug-run"]').trigger('click')
    await flushPromises()
    const preview = wrapper.find('[data-testid="gpt-auto-debug-preview"]')
    expect(preview.find('[data-testid="gpt-auto-context-diagnostics"]').exists()).toBe(true)
    expect(preview.find('[data-testid="gpt-auto-diagnostics-synthetic"]').exists()).toBe(true)
    expect(preview.find('[data-testid="gpt-auto-diagnostics-step-continues_request"]').exists()).toBe(true)
    const excerpts = wrapper.find('[data-testid="gpt-auto-debug-excerpts"]').text()
    expect(excerpts).toContain('fix it')
    expect(excerpts).toContain('Implement an LRU cache')
    expect(excerpts).toContain('--- FAIL: TestExpiry')
    expect(wrapper.find('[data-testid="gpt-auto-debug-criteria-target_model"]').text()).toContain('efficient general model')
    expect(setItem).not.toHaveBeenCalled()
    setItem.mockRestore()
  })

  it('rejects invalid JSON without calling the backend', async () => {
    wrapper = mountWith(GPTAutoStateDebugger)
    await wrapper.find('[data-testid="gpt-auto-debug-request"]').setValue('[1, 2')
    await wrapper.find('[data-testid="gpt-auto-debug-run"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-debug-error"]').text()).toBe('gptAuto.debugger.invalidJson')
    expect(mocks.api.debugState).not.toHaveBeenCalled()
  })
})

describe('GPTAutoConfigPanel context strategy', () => {
  it('saves the context strategy and agent steps', async () => {
    wrapper = mountWith(GPTAutoConfigPanel)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-context-strategy"]').setValue('recent_turns')
    await wrapper.find('[data-testid="gpt-auto-save"]').trigger('click')
    await flushPromises()
    expect(mocks.api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ classification_context_strategy: 'recent_turns', classification_agent_steps: 8 }))
  })
})

describe('GPT Auto Phase 2', () => {
  it('shows the runtime mode and versions reported by the backend', async () => {
    wrapper = mountWith(GPTAutoView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-phase-badge"]').text()).toBe('gptAuto.phaseBadge')
    expect(wrapper.find('[data-testid="gpt-auto-tab-live"]').exists()).toBe(true)
    wrapper.unmount()

    mocks.api.getMeta.mockResolvedValue({ ...meta, live_routing_active: true, phase: 'live', state_version: 'state-v3', classifier_schema: 'jev-questions-v2', model_pool_version: 'pool-v2' })
    wrapper = mountWith(GPTAutoView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-phase-badge"]').text()).toBe('gptAuto.phaseBadgeLive')
    expect(wrapper.find('[data-testid="gpt-auto-shadow-banner"]').text()).toContain('gptAuto.liveBanner.body')
    const versions = wrapper.find('[data-testid="gpt-auto-versions"]').text()
    expect(versions).toContain('state-v3')
    expect(versions).toContain('pool-v2')
  })

  it('keeps dynamic routing off by default and persists explicit per-turn settings', async () => {
    wrapper = mountWith(GPTAutoLivePanel)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-dynamic"]').exists()).toBe(true)
    await wrapper.find('[data-testid="dynamic-enabled"]').trigger('click')
    await wrapper.find('[data-testid="gpt-auto-live-save"]').trigger('click')
    await flushPromises()
    const cfg = mocks.api.updateConfig.mock.calls[0][0]
    expect(cfg.live.dynamic_routing.enabled).toBe(true)
    expect(cfg.live.dynamic_routing.stateless_strategy).toBe('per_request')
    expect(cfg.live.dynamic_routing.downgrade_stability_count).toBe(2)
    expect(cfg.live.live_enabled).toBe(false)
  })

  it('requires confirmation to enable live routing and saves only through a fresh config', async () => {
    wrapper = mountWith(GPTAutoLivePanel)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-live-state"]').text()).toContain('gptAuto.live.stateOff')
    expect(wrapper.find('[data-testid="gpt-auto-live-cap-gpt-5.6-sol"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-auto-live-retired"]').text()).toContain('gptAuto.live.capabilities.retired')

    await wrapper.find('[data-testid="gpt-auto-live-toggle"]').trigger('click')
    await wrapper.find('[data-testid="gpt-auto-live-keys"]').setValue('12, 34, abc, -1')
    await wrapper.find('[data-testid="gpt-auto-live-save"]').trigger('click')
    await flushPromises()
    expect(mocks.api.updateConfig).not.toHaveBeenCalled()
    await wrapper.find('[data-testid="confirm-yes"]').trigger('click')
    await flushPromises()
    expect(mocks.api.updateConfig).toHaveBeenCalledTimes(1)
    const payload = mocks.api.updateConfig.mock.calls[0][0]
    expect(payload.live.live_enabled).toBe(true)
    expect(payload.live.internal_only).toBe(true)
    expect(payload.live.allowed_api_keys).toEqual([12, 34])
    expect(payload.classifier_api_key).toBe('')
    expect(payload.model_profiles.map((p: { model: string }) => p.model)).toEqual(['gpt-5.6-luna', 'gpt-6-luna', 'gpt-6.1-sol'])
    // Shadow and live comparison uses server-side mode filters.
    expect(mocks.api.getOverview).toHaveBeenCalledWith(expect.objectContaining({ mode: 'shadow' }))
    expect(mocks.api.getOverview).toHaveBeenCalledWith(expect.objectContaining({ mode: 'live' }))
  })

  it('filters by mode and exports through the server with the current filters', async () => {
    mocks.api.listDecisions.mockResolvedValue({ items: [{ ...decision(), mode: 'live', request_status: 'succeeded', actual_model: 'gpt-auto' }], total: 1, page: 1, page_size: 20, pages: 1 })
    mocks.api.countExport.mockResolvedValue({ rows: 1200001, truncated: true, max_rows: 1000000 })
    mocks.api.exportDecisions.mockResolvedValue({ blob: new Blob(['x']), filename: 'gpt-auto-routing-admin-a-b.jsonl', rows: null, truncated: false })
    const createObjectURL = vi.fn(() => 'blob:test')
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() })

    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-mode-7"]').text()).toContain('gptAuto.live.modes.live')
    await wrapper.find('[data-testid="gpt-auto-logs-mode"]').setValue('live')
    await flushPromises()
    expect(mocks.api.listDecisions).toHaveBeenLastCalledWith(expect.objectContaining({ mode: 'live' }))

    await wrapper.find('[data-testid="gpt-auto-export-open"]').trigger('click')
    await flushPromises()
    expect((wrapper.find('[data-testid="gpt-auto-export-mode"]').element as HTMLSelectElement).value).toBe('live')
    await wrapper.find('[data-testid="gpt-auto-export-format"]').setValue('jsonl')
    await wrapper.find('[data-testid="gpt-auto-export-download"]').trigger('click')
    await flushPromises()
    expect(mocks.api.countExport).toHaveBeenCalledWith(expect.objectContaining({ mode: 'live' }))
    expect(mocks.api.exportDecisions).toHaveBeenCalledWith(expect.objectContaining({ format: 'jsonl', mode: 'live' }))
    expect(wrapper.find('[data-testid="gpt-auto-export-summary"]').text()).toContain('gptAuto.export.truncated')
    expect(createObjectURL).toHaveBeenCalled()
  })

  it('surfaces export validation errors instead of downloading', async () => {
    mocks.api.countExport.mockRejectedValue(new Error('bad range'))
    wrapper = mountWith(GPTAutoDecisionLog)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-export-open"]').trigger('click')
    await wrapper.find('[data-testid="gpt-auto-export-download"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-export-error"]').text()).toBe('gptAuto.export.failed')
    expect(mocks.api.exportDecisions).not.toHaveBeenCalled()
  })
})
