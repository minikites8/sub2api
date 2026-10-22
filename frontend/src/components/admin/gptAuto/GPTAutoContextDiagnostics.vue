<template>
  <section class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="gpt-auto-context-diagnostics">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.diagnostics.title') }}</h3>
      <span v-if="synthetic" class="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-300" data-testid="gpt-auto-diagnostics-synthetic">{{ t('gptAuto.diagnostics.synthetic') }}</span>
    </div>

    <p v-if="availability !== 'available'" class="rounded bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300" data-testid="gpt-auto-diagnostics-unavailable">
      {{ t('gptAuto.diagnostics.unavailable') }}
      <span v-if="availability === 'none'" class="block text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.noContextMetadata') }}</span>
    </p>

    <!-- A. Context summary -->
    <dl class="grid grid-cols-2 gap-3 text-xs sm:grid-cols-3 lg:grid-cols-4" data-testid="gpt-auto-diagnostics-summary">
      <div v-for="fact in summary" :key="fact.key" :data-testid="`gpt-auto-diagnostics-fact-${fact.key}`">
        <dt class="text-gray-500 dark:text-gray-400">{{ fact.label }}</dt>
        <dd class="break-words font-medium text-gray-900 dark:text-white" :class="fact.mono ? 'font-mono' : ''">{{ fact.value }}</dd>
      </div>
    </dl>
    <p v-if="versionMismatch" class="text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-auto-diagnostics-version-note">{{ t('gptAuto.diagnostics.versionDiffers', versionMismatch) }}</p>

    <template v-if="diagnostics">
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span class="rounded-full px-2 py-0.5 font-medium" :class="completenessClass" data-testid="gpt-auto-diagnostics-completeness">{{ t(`gptAuto.diagnostics.completeness.${diagnostics.completeness}`) }}</span>
        <span v-for="reason in diagnostics.completeness_reasons ?? []" :key="reason" class="rounded-full bg-gray-100 px-2 py-0.5 text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ codeLabel(reason) }}</span>
      </div>
      <p class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.completenessHint') }}</p>

      <!-- B. Budget allocation -->
      <div v-if="segments" data-testid="gpt-auto-diagnostics-budget">
        <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.budgetTitle', { budget: formatNumber(context?.budget_chars) }) }}</p>
        <div class="flex h-3 w-full overflow-hidden rounded bg-gray-100 dark:bg-dark-700" role="img" :aria-label="budgetAria">
          <div v-for="segment in visibleSegments" :key="segment.key" class="h-full" :class="segmentClass[segment.key]" :style="{ width: `${(segment.share * 100).toFixed(2)}%` }" :title="`${t(`gptAuto.diagnostics.segments.${segment.key}`)}: ${formatNumber(segment.chars)}`" :data-testid="`gpt-auto-diagnostics-segment-${segment.key}`"></div>
        </div>
        <ul class="mt-2 grid grid-cols-1 gap-x-4 gap-y-1 text-xs sm:grid-cols-2 lg:grid-cols-3">
          <li v-for="segment in segments" :key="segment.key" class="flex items-center gap-2">
            <span class="inline-block h-2.5 w-2.5 shrink-0 rounded-sm" :class="segmentClass[segment.key]"></span>
            <span class="text-gray-600 dark:text-gray-300">{{ t(`gptAuto.diagnostics.segments.${segment.key}`) }}</span>
            <span class="ml-auto tabular-nums text-gray-900 dark:text-white">{{ formatNumber(segment.chars) }}</span>
          </li>
        </ul>
        <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.budgetHint') }}</p>
      </div>
      <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.budgetUnavailable') }}</p>

      <!-- C. Selection timeline -->
      <div v-if="timeline.length" data-testid="gpt-auto-diagnostics-timeline">
        <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.timelineTitle') }}</p>
        <ol class="space-y-1.5 border-l border-gray-200 pl-3 dark:border-dark-600">
          <li v-for="step in timeline" :key="step.section" class="text-xs" :data-testid="`gpt-auto-diagnostics-step-${step.section}`">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ t(`gptAuto.diagnostics.sections.${step.section}`) }}</span>
              <span class="rounded-full px-2 py-0.5 text-[11px]" :class="statusClass(step.status)">{{ t(`gptAuto.diagnostics.statuses.${step.status}`) }}</span>
            </div>
            <p class="text-gray-500 dark:text-gray-400">{{ stepDetail(step) }}</p>
            <p v-if="step.section === 'agent_activity' && agent" class="text-gray-500 dark:text-gray-400" data-testid="gpt-auto-diagnostics-agent">
              {{ t('gptAuto.diagnostics.agentDetail', { steps: agent.steps_detected, calls: agent.tool_calls_detected, results: agent.tool_results_detected, limit: agent.step_limit }) }}
              <span v-if="agent.tool_failures_detected" class="block text-amber-700 dark:text-amber-300">{{ t('gptAuto.diagnostics.toolFailures', { detected: agent.tool_failures_detected, included: agent.failure_summaries_included, trailing: agent.trailing_failures }) }}</span>
            </p>
          </li>
        </ol>
      </div>

      <!-- Item accounting: why items did not reach the state -->
      <div v-if="items" data-testid="gpt-auto-diagnostics-items">
        <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.itemsTitle') }}</p>
        <dl class="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
          <div v-for="row in itemRows" :key="row.key" :title="t(`gptAuto.diagnostics.itemHints.${row.key}`)">
            <dt class="text-gray-500 dark:text-gray-400">{{ t(`gptAuto.diagnostics.itemLabels.${row.key}`) }}</dt>
            <dd class="tabular-nums font-medium text-gray-900 dark:text-white">{{ formatNumber(row.value) }}</dd>
          </div>
        </dl>
      </div>

      <!-- D. Warnings -->
      <ul v-if="warnings.length" class="space-y-1" data-testid="gpt-auto-diagnostics-warnings">
        <li v-for="warning in warnings" :key="warning" class="rounded bg-amber-50/60 px-3 py-1.5 text-xs text-amber-800 dark:bg-amber-900/10 dark:text-amber-200" :data-testid="`gpt-auto-diagnostics-warning-${warning}`">{{ t(`gptAuto.diagnostics.warnings.${warning}`) }}</li>
      </ul>
      <p v-if="diagnostics.notes?.length" class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.notesTitle') }}: {{ diagnostics.notes.map(codeLabel).join(' · ') }}</p>
    </template>

    <!-- E. Classification vs routing policy -->
    <div v-if="classification && decision" class="overflow-x-auto" data-testid="gpt-auto-diagnostics-comparison">
      <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.comparisonTitle') }}</p>
      <table class="min-w-full text-xs">
        <thead>
          <tr class="text-left text-gray-500 dark:text-gray-400">
            <th class="py-1 pr-3"></th>
            <th class="py-1 pr-3">{{ t('gptAuto.diagnostics.jevColumn') }}</th>
            <th class="py-1">{{ t('gptAuto.diagnostics.policyColumn') }}</th>
          </tr>
        </thead>
        <tbody class="text-gray-900 dark:text-gray-100">
          <tr>
            <td class="py-1 pr-3 text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.model') }}</td>
            <td class="py-1 pr-3 font-mono">{{ decision.jev_model || '—' }} <span class="text-gray-500">({{ formatPercent(classification.model_confidence, 0) }})</span></td>
            <td class="py-1 font-mono" :class="decision.final_model !== decision.jev_model ? 'text-amber-600 dark:text-amber-400' : ''">{{ decision.final_model || '—' }}</td>
          </tr>
          <tr>
            <td class="py-1 pr-3 text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.effort') }}</td>
            <td class="py-1 pr-3">{{ labels.effort(decision.jev_effort) }} <span class="text-gray-500">({{ formatPercent(classification.effort_confidence, 0) }})</span></td>
            <td class="py-1" :class="decision.final_effort !== decision.jev_effort ? 'text-amber-600 dark:text-amber-400' : ''">{{ labels.effort(decision.final_effort) }}</td>
          </tr>
          <tr>
            <td class="py-1 pr-3 text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.complexity') }}</td>
            <td class="py-1 pr-3 tabular-nums">{{ classification.complexity.toFixed(2) }} / 4</td>
            <td class="py-1 text-gray-500">{{ t('gptAuto.diagnostics.notPolicy') }}</td>
          </tr>
          <tr>
            <td class="py-1 pr-3 text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.category') }}</td>
            <td class="py-1 pr-3">{{ labels.category(classification.category) }}</td>
            <td class="py-1 text-gray-500">{{ t('gptAuto.diagnostics.notPolicy') }}</td>
          </tr>
          <tr>
            <td class="py-1 pr-3 align-top text-gray-500 dark:text-gray-400">{{ t('gptAuto.diagnostics.candidates') }}</td>
            <td class="py-1 pr-3 font-mono" data-testid="gpt-auto-diagnostics-probabilities">
              <span v-for="[model, probability] in topProbabilities" :key="model" class="block">{{ model }} {{ formatPercent(probability, 0) }}</span>
              <span v-if="!topProbabilities.length" class="text-gray-500">{{ t('gptAuto.common.noData') }}</span>
            </td>
            <td class="py-1 align-top">
              <span v-for="(adjustment, index) in rules.adjustments" :key="index" class="block">{{ labels.rule(adjustment.rule) }}<span v-if="adjustment.from || adjustment.to" class="ml-1 font-mono text-gray-500">{{ adjustment.from || '—' }} → {{ adjustment.to || '—' }}</span></span>
              <span v-if="!rules.adjustments.length" class="text-gray-500">{{ t('gptAuto.diagnostics.noPolicyChange') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GPTAutoClassification, GPTAutoContextInfo, GPTAutoContextSelectionStep, GPTAutoPolicyDecision, GPTAutoSelectionStatus } from '@/api/admin/gptAuto'
import { formatNumber, formatPercent, sortedProbabilities, splitPolicyRules } from '@/utils/gptAuto'
import { contextBudgetSegments, contextTimeline, contextWarnings, diagnosticsAvailability, shortHash, type BudgetSegmentKey } from '@/utils/gptAutoContextDiagnostics'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const props = withDefaults(defineProps<{
  context?: GPTAutoContextInfo | null
  classification?: GPTAutoClassification | null
  decision?: GPTAutoPolicyDecision | null
  /** The decision's classifier config hash ('' = before classifier versioning). */
  classifierHash?: string | null
  /** schema_version of the stored classifier config; null when it is not available. */
  classifierSchema?: string | null
  /** Versions reported by the backend node serving this page (GET /meta). */
  runtime?: { state_version?: string; classifier_schema?: string; diagnostics_version?: string } | null
  /** Debugger mode: the input is an administrator-provided synthetic fixture. */
  synthetic?: boolean
}>(), { context: null, classification: null, decision: null, classifierHash: null, classifierSchema: null, runtime: null, synthetic: false })

const { t, te } = useI18n()
const labels = useGPTAutoLabels()

const availability = computed(() => diagnosticsAvailability(props.context))
const diagnostics = computed(() => props.context?.diagnostics ?? null)
const items = computed(() => diagnostics.value?.items ?? null)
const agent = computed(() => diagnostics.value?.agent ?? null)
const segments = computed(() => contextBudgetSegments(props.context))
const visibleSegments = computed(() => (segments.value ?? []).filter((segment) => segment.chars > 0))
const timeline = computed(() => contextTimeline(diagnostics.value))
const warnings = computed(() => contextWarnings(props.context))
const rules = computed(() => (props.decision ? splitPolicyRules(props.decision) : { adjustments: [], warnings: [] }))
const topProbabilities = computed(() => sortedProbabilities(props.classification?.model_probabilities).slice(0, 4))

const unknown = computed(() => t('gptAuto.diagnostics.unknown'))

/** Schema of the classifier questions: stored config, else v1 for unversioned rows, else unknown. */
const schema = computed(() => {
  if (props.classifierSchema) return props.classifierSchema
  if (props.classifierHash === '') return t('gptAuto.diagnostics.schemaLegacy')
  return unknown.value
})

const summary = computed(() => {
  const context = props.context
  const d = diagnostics.value
  const followUp = d?.follow_up
  const facts: { key: string; label: string; value: string | number; mono?: boolean }[] = [
    { key: 'strategy', label: t('gptAuto.diagnostics.facts.strategy'), value: context?.strategy ? t(`gptAuto.config.classifier.strategies.${context.strategy}`) : unknown.value },
    { key: 'stateVersion', label: t('gptAuto.diagnostics.facts.stateVersion'), value: context?.state_version || t('gptAuto.diagnostics.stateLegacy'), mono: true },
    { key: 'classifierSchema', label: t('gptAuto.diagnostics.facts.classifierSchema'), value: schema.value, mono: true },
  ]
  if (props.classifierHash !== null) {
    facts.push({ key: 'classifierHash', label: t('gptAuto.diagnostics.facts.classifierHash'), value: shortHash(props.classifierHash) || t('gptAuto.detail.classifierLegacy'), mono: true })
  }
  facts.push({ key: 'diagnosticsVersion', label: t('gptAuto.diagnostics.facts.diagnosticsVersion'), value: d?.diagnostics_version || '—', mono: true })
  if (!context) return facts
  const scanned = d ? d.scan_status === 'scanned' : true
  facts.push(
    { key: 'items', label: t('gptAuto.diagnostics.facts.items'), value: scanned ? formatNumber(context.items_total) : unknown.value },
    { key: 'selected', label: t('gptAuto.diagnostics.facts.selected'), value: items.value ? `${formatNumber(items.value.selected_messages)} · ${t('gptAuto.diagnostics.turnsOf', { included: context.context_turns_included, available: context.context_turns_available })}` : `${t('gptAuto.diagnostics.turnsOf', { included: context.context_turns_included, available: context.context_turns_available })}` },
    { key: 'agent', label: t('gptAuto.diagnostics.facts.agent'), value: agent.value ? t('gptAuto.diagnostics.agentSteps', { included: context.agent_events_included, detected: agent.value.steps_detected }) : d ? t('gptAuto.diagnostics.none') : `${context.agent_events_included}` },
    { key: 'followUp', label: t('gptAuto.diagnostics.facts.followUp'), value: followUp ? t(`gptAuto.diagnostics.followUp.${followUp.resolution}`) : t(`gptAuto.debugger.kinds.${context.latest_user_kind}`) },
    { key: 'latestUser', label: t('gptAuto.diagnostics.facts.latestUser'), value: context.latest_user_index >= 0 ? t('gptAuto.diagnostics.latestFound', { index: context.latest_user_index }) : t('gptAuto.diagnostics.latestMissing') },
    { key: 'history', label: t('gptAuto.diagnostics.facts.history'), value: items.value ? (items.value.conversation_length_known ? t('gptAuto.diagnostics.historyInRequest') : t('gptAuto.diagnostics.historyExternal')) : unknown.value },
    { key: 'truncated', label: t('gptAuto.diagnostics.facts.truncated'), value: d ? (d.context_truncated ? t('gptAuto.common.yes') : t('gptAuto.common.no')) : (context.latest_user_truncated ? t('gptAuto.common.yes') : unknown.value) },
    { key: 'used', label: t('gptAuto.diagnostics.facts.used'), value: `${formatNumber(context.used_chars)} / ${formatNumber(context.budget_chars)}` },
  )
  if (d?.state_chars !== undefined) facts.push({ key: 'stateChars', label: t('gptAuto.diagnostics.facts.stateChars'), value: formatNumber(d.state_chars) })
  return facts
})

const versionMismatch = computed(() => {
  const runtime = props.runtime
  const recorded = props.context?.state_version
  if (!runtime?.state_version || !recorded || runtime.state_version === recorded) return null
  return { recorded, running: runtime.state_version }
})

const itemRows = computed(() => {
  const counts = items.value
  if (!counts) return []
  return ([
    ['eligible', counts.eligible], ['represented', counts.represented], ['not_selected', counts.not_selected], ['budget_dropped', counts.budget_dropped],
    ['budget_truncated', counts.budget_truncated], ['privacy', counts.privacy], ['external', counts.external_references + counts.compacted],
    ['unsupported', counts.unsupported], ['empty', counts.empty], ['with_images', counts.with_images],
  ] as [string, number][]).map(([key, value]) => ({ key, value }))
})

const segmentClass: Record<BudgetSegmentKey, string> = {
  latest: 'bg-primary-500',
  continues: 'bg-violet-500',
  original: 'bg-sky-500',
  agent: 'bg-amber-500',
  recent: 'bg-emerald-500',
  unused: 'bg-gray-200 dark:bg-dark-500',
}

const budgetAria = computed(() => (segments.value ?? []).map((segment) => `${t(`gptAuto.diagnostics.segments.${segment.key}`)} ${segment.chars}`).join(', '))

const completenessClass = computed(() => {
  switch (diagnostics.value?.completeness) {
    case 'sufficient': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'partial': return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'important_unavailable': return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default: return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
})

function statusClass(status: GPTAutoSelectionStatus) {
  switch (status) {
    case 'included': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'truncated': return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'budget_exhausted': return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default: return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

function stepDetail(step: GPTAutoContextSelectionStep): string {
  const parts: string[] = []
  if (step.status === 'unavailable') {
    parts.push(t(step.section === 'external_history' ? 'gptAuto.diagnostics.externalUnavailable' : 'gptAuto.diagnostics.notInRequest'))
  } else if (step.status !== 'budget_exhausted' || step.chars > 0) {
    parts.push(t('gptAuto.diagnostics.chars', { chars: formatNumber(step.chars) }))
  }
  const unit = step.section === 'agent_activity' ? 'events' : 'messages'
  if (step.items) parts.push(t(`gptAuto.diagnostics.${unit}`, { count: step.items }))
  if (step.available !== undefined && step.available > 0) parts.push(t('gptAuto.diagnostics.available', { count: step.available }))
  if (step.dropped) parts.push(t('gptAuto.diagnostics.dropped', { count: step.dropped }))
  return parts.join(' · ')
}

/** Limitation codes have their own labels; diagnostic notes live under diagnostics.notes. */
function codeLabel(code: string): string {
  const note = `gptAuto.diagnostics.noteLabels.${code}`
  return te(note) ? t(note) : labels.limitation(code)
}
</script>
