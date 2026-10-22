import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import GPTAutoContextDiagnostics from '../GPTAutoContextDiagnostics.vue'
import type { GPTAutoClassification, GPTAutoContextDiagnostics as Diagnostics, GPTAutoContextInfo, GPTAutoPolicyDecision } from '@/api/admin/gptAuto'
import { contextBudgetSegments, contextTimeline, contextWarnings, diagnosticsAvailability, shortHash } from '@/utils/gptAutoContextDiagnostics'

// Synthetic diagnostics only; no customer data.

function diagnostics(overrides: Partial<Diagnostics> = {}): Diagnostics {
  return {
    diagnostics_version: 'context-diagnostics-v1',
    scan_status: 'scanned',
    items: {
      eligible: 1205, represented: 9, selected_messages: 4, budget_dropped: 0, budget_truncated: 1, not_selected: 1196,
      privacy: 2, unsupported: 0, external_references: 0, compacted: 0, empty: 0, with_images: 0, conversation_length_known: true,
    },
    selection: [
      { section: 'latest_user_request', status: 'included', chars: 8, items: 1 },
      { section: 'continues_request', status: 'truncated', chars: 1000, items: 1 },
      { section: 'agent_activity', status: 'included', chars: 600, items: 5 },
      { section: 'original_request', status: 'not_applicable', chars: 0 },
      { section: 'history_summary', status: 'not_applicable', chars: 0 },
      { section: 'recent_context', status: 'included', chars: 1400, items: 3, available: 60 },
      { section: 'external_history', status: 'not_applicable', chars: 0 },
    ],
    context_truncated: true,
    follow_up: { resolution: 'linked_request', short_message: true, original_task_available: true, agent_activity_available: true },
    agent: {
      scope: 'before_follow_up', step_limit: 8, steps_detected: 12, tool_calls_detected: 6, tool_results_detected: 6, tool_failures_detected: 2,
      trailing_failures: 1, failure_summaries_included: 2, events_dropped_for_budget: 0, event_kinds: { tool_call: 3, tool_result_error: 2 },
      has_tool_result_context: true, has_progress_note: false,
    },
    completeness: 'partial',
    completeness_reasons: ['earlier_context_omitted'],
    state_chars: 3218,
    ...overrides,
  }
}

function context(overrides: Partial<GPTAutoContextInfo> = {}): GPTAutoContextInfo {
  return {
    state_version: 'state-v3', strategy: 'task_aware', items_total: 1207, scanned_bytes: 50000, latest_user_index: 1206,
    latest_user_kind: 'follow_up', latest_user_truncated: false, follow_up_anchor_index: 1190, original_request_index: -1,
    context_turns_available: 60, context_turns_included: 3, agent_events_included: 5, agent_events_omitted: 7,
    budget_chars: 4000, used_chars: 3008, limitations: ['earlier_context_omitted', 'tool_outputs_summarized'],
    diagnostics: diagnostics(),
    ...overrides,
  }
}

// Tests use the runtime-only vue-i18n build (no message compiler), so
// messages become functions that perform named interpolation.
function messageFunctions(value: unknown): unknown {
  if (typeof value === 'string') {
    return (ctx: { named: (key: string) => unknown }) => value.replace(/\{(\w+)\}/g, (_, key: string) => String(ctx.named(key)))
  }
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).map(([key, child]) => [key, messageFunctions(child)]))
}

function i18n(locale: 'en' | 'zh' = 'en') {
  return createI18n({ legacy: false, locale, fallbackLocale: 'en', messages: { en: messageFunctions(en) as typeof en, zh: messageFunctions(zh) as typeof en } })
}

function render(props: Record<string, unknown>, locale: 'en' | 'zh' = 'en') {
  return mount(GPTAutoContextDiagnostics, { props, global: { plugins: [i18n(locale)] } })
}

describe('GPTAutoContextDiagnostics', () => {
  it('renders the summary, budget, timeline and warnings from recorded facts', () => {
    const wrapper = render({ context: context(), classifierHash: 'abcdef0123456789ff', classifierSchema: 'jev-questions-v2' })
    const summary = wrapper.find('[data-testid="gpt-auto-diagnostics-summary"]').text()
    expect(summary).toContain('Task-aware')
    expect(summary).toContain('state-v3')
    expect(summary).toContain('jev-questions-v2')
    expect(summary).toContain('abcdef0123')
    expect(summary).not.toContain('abcdef0123456789ff')
    expect(summary).toContain('1,207')
    expect(summary).toContain('Follow-up · earlier request linked')
    expect(summary).toContain('5 events of 12 steps')
    expect(summary).toContain('3,218')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-completeness"]').text()).toBe('Partial context observed')

    // Budget bar: widths are the recorded chars over the budget.
    const width = (key: string) => (wrapper.find(`[data-testid="gpt-auto-diagnostics-segment-${key}"]`).element as HTMLElement).style.width
    expect(width('latest')).toBe('0.2%')
    expect(width('continues')).toBe('25%')
    expect(width('agent')).toBe('15%')
    expect(width('recent')).toBe('35%')
    expect(width('unused')).toBe('24.8%')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-segment-original"]').exists()).toBe(false)

    // Timeline: server order, not-applicable steps hidden.
    const steps = wrapper.findAll('[data-testid^="gpt-auto-diagnostics-step-"]').map((step) => step.attributes('data-testid'))
    expect(steps).toEqual([
      'gpt-auto-diagnostics-step-latest_user_request', 'gpt-auto-diagnostics-step-continues_request',
      'gpt-auto-diagnostics-step-agent_activity', 'gpt-auto-diagnostics-step-recent_context',
    ])
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-step-continues_request"]').text()).toContain('Truncated')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-agent"]').text()).toContain('2 tool failure(s) detected, 2 summarised')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-step-recent_context"]').text()).toContain('60 available')

    const warnings = wrapper.findAll('[data-testid^="gpt-auto-diagnostics-warning-"]').map((item) => item.attributes('data-testid'))
    expect(warnings).toEqual([
      'gpt-auto-diagnostics-warning-context_truncated', 'gpt-auto-diagnostics-warning-continues_task', 'gpt-auto-diagnostics-warning-tool_failures',
    ])
    expect(wrapper.text()).not.toMatch(/incorrect|wrong/i)
  })

  it('says diagnostics are unavailable for historical decisions and invents nothing', () => {
    const legacy = render({ context: context({ diagnostics: undefined }), classifierHash: 'abc' })
    expect(legacy.find('[data-testid="gpt-auto-diagnostics-unavailable"]').text()).toContain('Diagnostics unavailable — created before context diagnostics were enabled.')
    expect(legacy.find('[data-testid="gpt-auto-diagnostics-budget"]').exists()).toBe(false)
    expect(legacy.find('[data-testid="gpt-auto-diagnostics-timeline"]').exists()).toBe(false)
    expect(legacy.find('[data-testid="gpt-auto-diagnostics-warnings"]').exists()).toBe(false)
    expect(legacy.find('[data-testid="gpt-auto-diagnostics-fact-history"]').text()).toContain('Unknown')

    const none = render({ context: null, classifierHash: '' })
    expect(none.find('[data-testid="gpt-auto-diagnostics-unavailable"]').text()).toContain('state v1/v2')
    expect(none.find('[data-testid="gpt-auto-diagnostics-fact-classifierSchema"]').text()).toContain('jev-questions-v1')
    expect(none.find('[data-testid="gpt-auto-diagnostics-fact-items"]').exists()).toBe(false)
  })

  it('shows unknown counts for a request that was not scanned', () => {
    const skipped = diagnostics({ scan_status: 'skipped_too_large', items: undefined, selection: undefined, agent: undefined, completeness: 'unable_to_determine', completeness_reasons: ['request_too_large'], context_truncated: false, state_chars: undefined })
    const wrapper = render({ context: context({ diagnostics: skipped, items_total: 0 }) })
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-fact-items"]').text()).toContain('Unknown')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-items"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-completeness"]').text()).toBe('Unable to determine')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-warning-not_scanned"]').exists()).toBe(true)
  })

  it('separates Jev classification from downstream routing policy', () => {
    const classification = {
      model: 'gpt-6.1-sol', effort: 'high', complexity: 2, category: 'coding_debugging', model_confidence: 0.41, effort_confidence: 0.6,
      complexity_confidence: 0.5, category_confidence: 0.8, model_probabilities: { 'gpt-6.1-sol': 0.41, 'gpt-6-luna': 0.39 },
    } as unknown as GPTAutoClassification
    const decision = {
      status: 'adjusted', jev_model: 'gpt-6.1-sol', jev_effort: 'high', final_model: 'gpt-6-luna', final_effort: 'high',
      adjustments: [{ rule: 'routing_weight_tie_break', from: 'gpt-6.1-sol', to: 'gpt-6-luna' }], warnings: [], candidates: [],
    } as unknown as GPTAutoPolicyDecision
    const wrapper = render({ context: context(), classification, decision })
    const table = wrapper.find('[data-testid="gpt-auto-diagnostics-comparison"]')
    expect(table.text()).toContain('Jev (classification)')
    expect(table.text()).toContain('Policy (downstream routing)')
    expect(table.find('.text-amber-600').text()).toBe('gpt-6-luna')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-probabilities"]').text()).toContain('gpt-6.1-sol 41%')
    expect(table.text()).toContain('gpt-6.1-sol → gpt-6-luna')
  })

  it('notes when the decision was recorded by another state version', () => {
    const wrapper = render({ context: context({ state_version: 'state-v3' }), runtime: { state_version: 'state-v4' } })
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-version-note"]').text()).toContain('state-v4')
    const same = render({ context: context(), runtime: { state_version: 'state-v3' } })
    expect(same.find('[data-testid="gpt-auto-diagnostics-version-note"]').exists()).toBe(false)
  })

  it('labels synthetic fixtures and renders in Chinese', () => {
    const wrapper = render({ context: context(), synthetic: true }, 'zh')
    expect(wrapper.text()).toContain('分类上下文诊断')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-synthetic"]').text()).toBe('管理员提供的测试样例')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-completeness"]').text()).toBe('观察到的上下文不完整')
  })

  it('styles every element for light and dark themes and narrow screens', () => {
    const wrapper = render({ context: context() })
    expect(wrapper.find('[data-testid="gpt-auto-context-diagnostics"]').classes()).toContain('dark:border-dark-600')
    expect(wrapper.find('[data-testid="gpt-auto-diagnostics-segment-unused"]').classes()).toContain('dark:bg-dark-500')
    for (const badge of wrapper.findAll('[data-testid^="gpt-auto-diagnostics-step-"] span.rounded-full')) {
      expect(badge.classes().some((name) => name.startsWith('dark:'))).toBe(true)
    }
    const grid = wrapper.find('[data-testid="gpt-auto-diagnostics-summary"]').classes()
    expect(grid).toContain('grid-cols-2')
    expect(grid).toContain('sm:grid-cols-3')
  })
})

describe('context diagnostics helpers', () => {
  it('reports availability without guessing', () => {
    expect(diagnosticsAvailability(context())).toBe('available')
    expect(diagnosticsAvailability(context({ diagnostics: undefined }))).toBe('context_only')
    expect(diagnosticsAvailability(undefined)).toBe('none')
    expect(contextBudgetSegments(context({ diagnostics: undefined }))).toBeNull()
    expect(contextBudgetSegments(context({ budget_chars: 0 }))).toBeNull()
    expect(contextWarnings(context({ diagnostics: undefined }))).toEqual([])
    expect(contextTimeline(undefined)).toEqual([])
    expect(shortHash('')).toBe('')
    expect(shortHash('0123456789abcdef', 6)).toBe('012345')
  })

  it('splits the budget by recorded section characters', () => {
    const segments = contextBudgetSegments(context({
      diagnostics: diagnostics({ selection: [
        { section: 'latest_user_request', status: 'included', chars: 1400 },
        { section: 'original_request', status: 'included', chars: 300 },
        { section: 'history_summary', status: 'included', chars: 200 },
      ] }),
      used_chars: 1900,
    }))!
    expect(Object.fromEntries(segments.map((segment) => [segment.key, segment.chars]))).toEqual({ latest: 1400, continues: 0, original: 500, agent: 0, recent: 0, unused: 2100 })
    expect(segments.reduce((sum, segment) => sum + segment.share, 0)).toBeCloseTo(1)
  })

  it('derives neutral warnings from limitations and resolution', () => {
    const unresolved = context({
      limitations: ['history_by_reference', 'images_not_forwarded', 'history_compacted'],
      diagnostics: diagnostics({ context_truncated: false, agent: undefined, completeness: 'important_unavailable', follow_up: { resolution: 'unresolved', short_message: true, original_task_available: false, agent_activity_available: false } }),
    })
    expect(contextWarnings(unresolved)).toEqual(['task_unresolved', 'previous_response_unavailable', 'external_history_unavailable', 'images_not_included', 'context_insufficient'])
  })
})

describe('context diagnostics locale coverage', () => {
  const keys = {
    sections: ['latest_user_request', 'continues_request', 'agent_activity', 'original_request', 'history_summary', 'recent_context', 'external_history'],
    statuses: ['included', 'truncated', 'budget_exhausted', 'unavailable', 'not_applicable'],
    followUp: ['not_follow_up', 'linked_request', 'agent_activity_only', 'unresolved', 'recent_turns_only'],
    completeness: ['sufficient', 'partial', 'important_unavailable', 'unable_to_determine'],
    segments: ['latest', 'continues', 'original', 'agent', 'recent', 'unused'],
    warnings: ['context_truncated', 'continues_task', 'task_unresolved', 'tool_failures', 'previous_response_unavailable', 'external_history_unavailable', 'images_not_included', 'context_insufficient', 'not_scanned'],
    noteLabels: ['unsupported_items', 'empty_user_messages', 'context_budget_exhausted', 'agent_events_trimmed', 'follow_up_unresolved', 'no_classifiable_content'],
    itemLabels: ['eligible', 'represented', 'not_selected', 'budget_dropped', 'budget_truncated', 'privacy', 'external', 'unsupported', 'empty', 'with_images'],
  }
  for (const [name, messages] of Object.entries({ en, zh })) {
    it(`has every diagnostics code in ${name}`, () => {
      const diagnosticsMessages = (messages as unknown as { gptAuto: { diagnostics: Record<string, Record<string, string>>; debugger: { examples: Record<string, string> } } }).gptAuto
      for (const [group, codes] of Object.entries(keys)) {
        for (const code of codes) expect(diagnosticsMessages.diagnostics[group]?.[code], `${group}.${code}`).toBeTruthy()
      }
      for (const code of keys.itemLabels) expect(diagnosticsMessages.diagnostics.itemHints[code]).toBeTruthy()
      for (const example of ['simple', 'routineCode', 'complexEngineering', 'followUp', 'agentFailure', 'longChat', 'previousResponse', 'multimodal']) {
        expect(diagnosticsMessages.debugger.examples[example], example).toBeTruthy()
      }
    })
  }
})
