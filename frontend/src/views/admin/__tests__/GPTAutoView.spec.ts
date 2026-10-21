import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GPTAutoView from '../GPTAutoView.vue'
import GPTAutoConfigPanel from '@/components/admin/gptAuto/GPTAutoConfigPanel.vue'
import GPTAutoDecisionLog from '@/components/admin/gptAuto/GPTAutoDecisionLog.vue'
import GPTAutoTestLab from '@/components/admin/gptAuto/GPTAutoTestLab.vue'
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
  },
  showSuccess: vi.fn(),
  showError: vi.fn(),
  replace: vi.fn(),
}))

vi.mock('@/api/admin/gptAuto', () => ({ gptAutoAPI: mocks.api, default: mocks.api }))
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
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
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
  worker_count: 2,
  queue_size: 4096,
  decision_retention_days: 30,
  global_min_margin: 0.3,
  profit_safety_buffer: 0.05,
  model_profiles: [profile('gpt-5.6-luna'), profile('gpt-5.6-sol'), profile('gpt-6-luna'), profile('gpt-6.1-sol', ['low', 'medium', 'high', 'xhigh', 'max'])],
  policy_version: 'shadow-v1',
  classifier_api_key_configured: true,
  classifier_api_key_masked: 'ts-k****1234',
})

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
})

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
    await wrapper.find('[data-testid="gpt-auto-weight-gpt-5.6-sol"]').setValue('300')
    await wrapper.find('[data-testid="gpt-auto-save"]').trigger('click')
    await flushPromises()

    expect(mocks.api.updateConfig).toHaveBeenCalledTimes(1)
    const payload = mocks.api.updateConfig.mock.calls[0][0]
    const byModel = Object.fromEntries(payload.model_profiles.map((p: { model: string }) => [p.model, p]))
    expect(byModel['gpt-6-luna'].billing_multiplier).toBe(1.7)
    expect(byModel['gpt-6-luna'].routing_weight).toBe(60)
    expect(byModel['gpt-5.6-sol'].routing_weight).toBe(300)
    expect(byModel['gpt-5.6-sol'].billing_multiplier).toBe(1.2)
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
