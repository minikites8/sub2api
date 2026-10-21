<template>
  <div class="space-y-4" data-testid="gpt-auto-decision-detail">
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.jevRecommendation') }}</p>
        <p class="mt-1 font-mono text-base font-semibold text-gray-900 dark:text-white">{{ decision.jev_model || '—' }}</p>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ labels.effort(decision.jev_effort) }}</p>
      </div>
      <div class="rounded-lg border p-3" :class="adjusted ? 'border-amber-300 bg-amber-50/50 dark:border-amber-700 dark:bg-amber-900/10' : 'border-emerald-300 bg-emerald-50/50 dark:border-emerald-700 dark:bg-emerald-900/10'">
        <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.policyRecommendation') }} · {{ labels.decisionStatus(decision.status) }}</p>
        <p class="mt-1 font-mono text-base font-semibold text-gray-900 dark:text-white" data-testid="gpt-auto-final-model">{{ decision.final_model || t('gptAuto.detail.noRoute') }}</p>
        <p class="text-sm text-gray-600 dark:text-gray-300" data-testid="gpt-auto-final-effort">{{ labels.effort(decision.final_effort) }}</p>
      </div>
    </div>

    <div v-if="!compact" class="grid grid-cols-2 gap-3 md:grid-cols-4">
      <div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.complexity') }}</p>
        <p class="font-medium text-gray-900 dark:text-white">{{ classification.complexity.toFixed(2) }} / 4 <span class="text-xs text-gray-500">({{ formatPercent(classification.complexity_confidence, 0) }})</span></p>
      </div>
      <div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.category') }}</p>
        <p class="font-medium text-gray-900 dark:text-white">{{ labels.category(classification.category) }} <span class="text-xs text-gray-500">({{ formatPercent(classification.category_confidence, 0) }})</span></p>
      </div>
      <div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.modelConfidence') }}</p>
        <p class="font-medium text-gray-900 dark:text-white">{{ formatPercent(classification.model_confidence, 0) }}</p>
      </div>
      <div>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.effortConfidence') }}</p>
        <p class="font-medium text-gray-900 dark:text-white">{{ formatPercent(classification.effort_confidence, 0) }}</p>
      </div>
    </div>
    <p v-if="!compact" class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.confidenceNote') }}</p>

    <div v-if="!compact" class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <div v-for="dist in distributions" :key="dist.key">
        <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ dist.title }}</p>
        <div v-if="dist.rows.length" class="space-y-1">
          <div v-for="[key, value] in dist.rows" :key="key" class="flex items-center gap-2">
            <span class="w-28 truncate text-xs text-gray-700 dark:text-gray-300" :title="dist.label(key)">{{ dist.label(key) }}</span>
            <div class="h-2 flex-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-700">
              <div class="h-full rounded bg-primary-500" :style="{ width: `${Math.round(value * 100)}%` }"></div>
            </div>
            <span class="w-10 text-right text-xs tabular-nums text-gray-500">{{ formatPercent(value, 0) }}</span>
          </div>
        </div>
        <p v-else class="text-xs text-gray-400">{{ t('gptAuto.common.noData') }}</p>
      </div>
    </div>

    <div>
      <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.candidates') }}</p>
      <div class="overflow-x-auto">
        <table class="min-w-full text-xs">
          <thead>
            <tr class="text-left text-gray-500 dark:text-gray-400">
              <th class="py-1 pr-3">{{ t('gptAuto.detail.candidateModel') }}</th>
              <th class="py-1 pr-3">{{ t('gptAuto.detail.candidateEligible') }}</th>
              <th class="py-1 pr-3">{{ t('gptAuto.detail.candidateProbability') }}</th>
              <th class="py-1 pr-3">{{ t('gptAuto.detail.candidateWeight') }}</th>
              <th class="py-1">{{ t('gptAuto.detail.candidateReasons') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="candidate in decision.candidates" :key="candidate.model" :class="candidate.model === decision.final_model ? 'font-semibold' : ''">
              <td class="py-1 pr-3 font-mono">{{ candidate.model }}</td>
              <td class="py-1 pr-3" :class="candidate.eligible ? 'text-emerald-600' : 'text-red-600'">{{ candidate.eligible ? t('gptAuto.common.yes') : t('gptAuto.common.no') }}</td>
              <td class="py-1 pr-3 tabular-nums">{{ formatPercent(candidate.probability, 0) }}</td>
              <td class="py-1 pr-3 tabular-nums">{{ candidate.routing_weight }}</td>
              <td class="py-1 text-gray-500">{{ (candidate.reasons ?? []).map(labels.rule).join(', ') || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="decision.adjustments?.length" data-testid="gpt-auto-adjustments">
      <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.adjustments') }}</p>
      <ul class="space-y-1">
        <li v-for="(adjustment, index) in decision.adjustments" :key="index" class="rounded bg-gray-50 px-3 py-1.5 text-xs dark:bg-dark-800">
          <span class="font-medium text-gray-900 dark:text-white">{{ labels.rule(adjustment.rule) }}</span>
          <span v-if="adjustment.from || adjustment.to" class="ml-2 font-mono text-gray-600 dark:text-gray-300">{{ adjustment.from || '—' }} → {{ adjustment.to || '—' }}</span>
          <span v-if="adjustment.detail" class="block text-gray-500">{{ adjustment.detail }}</span>
        </li>
      </ul>
    </div>

    <div v-if="decision.capability_warnings?.length" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
      <p class="font-medium">{{ t('gptAuto.detail.capabilityWarnings') }}</p>
      <p v-for="(warning, index) in decision.capability_warnings" :key="index">{{ labels.rule(warning) }}</p>
    </div>

    <div v-if="economics" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="gpt-auto-economics">
      <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.economics') }} <span class="ml-1 rounded-full bg-amber-50 px-2 py-0.5 text-[11px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('gptAuto.common.hypothetical') }}</span></p>
      <p v-if="!economics.pricing_available" class="mt-2 text-xs text-amber-600">{{ t('gptAuto.simulator.noPricing') }}</p>
      <dl v-else class="mt-2 grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
        <div><dt class="text-gray-500">{{ t('gptAuto.simulator.columns.revenue') }}</dt><dd class="tabular-nums">{{ formatUSD(economics.estimated_revenue) }} ({{ economics.billing_multiplier }}×)</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.simulator.columns.upstream') }}</dt><dd class="tabular-nums">{{ formatUSD(economics.estimated_upstream_cost) }} · {{ economics.upstream_channel || labels.costSource(economics.upstream_cost_source) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.simulator.columns.classifier') }}</dt><dd class="tabular-nums">{{ formatUSD(economics.estimated_classifier_cost, 8) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.simulator.columns.margin') }}</dt><dd class="tabular-nums" :class="economics.below_target ? 'text-red-600' : 'text-emerald-600'">{{ formatPercent(economics.estimated_gross_margin) }} / {{ formatPercent(economics.target_margin) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.listPrice') }}</dt><dd class="tabular-nums">${{ economics.list_input_per_m }} / ${{ economics.list_output_per_m }} {{ t('gptAuto.detail.perM') }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.upstreamPrice') }}</dt><dd class="tabular-nums">${{ economics.upstream_input_per_m }} / ${{ economics.upstream_output_per_m }} {{ t('gptAuto.detail.perM') }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.tokenEstimate') }}</dt><dd class="tabular-nums">{{ formatNumber(economics.estimated_input_tokens) }} / {{ formatNumber(Math.round(economics.estimated_output_tokens)) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.simulator.columns.basis') }}</dt><dd>{{ labels.outputSource(economics.output_estimate_source) }}</dd></div>
      </dl>
    </div>

    <div v-if="showActual" class="rounded-lg border border-sky-200 p-3 dark:border-sky-800/60" data-testid="gpt-auto-actual-usage">
      <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.detail.actualProduction') }} <span class="ml-1 rounded-full bg-sky-50 px-2 py-0.5 text-[11px] text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">{{ t('gptAuto.detail.actualBadge') }}</span></p>
      <p v-if="!actualUsage" class="mt-2 text-xs text-gray-500">{{ t('gptAuto.detail.actualMissing') }}</p>
      <dl v-else class="mt-2 grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualModel') }}</dt><dd class="font-mono">{{ actualUsage.model }}<span v-if="actualUsage.upstream_model" class="block text-gray-500">→ {{ actualUsage.upstream_model }}</span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualEffort') }}</dt><dd>{{ actualUsage.reasoning_effort ? labels.effort(actualUsage.reasoning_effort) : '—' }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualTokens') }}</dt><dd class="tabular-nums">{{ formatNumber(actualUsage.input_tokens) }} / {{ formatNumber(actualUsage.output_tokens) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualCharged') }}</dt><dd class="tabular-nums">{{ formatUSD(actualUsage.charged_cost) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualListCost') }}</dt><dd class="tabular-nums">{{ formatUSD(actualUsage.list_cost) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.actualAccountCost') }}</dt><dd class="tabular-nums">{{ formatUSD(actualUsage.account_cost) }}</dd></div>
      </dl>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GPTAutoActualUsage, GPTAutoClassification, GPTAutoPolicyDecision } from '@/api/admin/gptAuto'
import { formatNumber, formatPercent, formatUSD, sortedProbabilities } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const props = withDefaults(defineProps<{
  classification: GPTAutoClassification
  decision: GPTAutoPolicyDecision
  compact?: boolean
  /** Show the real production outcome (stored decisions only, not the test lab). */
  showActual?: boolean
  actualUsage?: GPTAutoActualUsage | null
}>(), { compact: false, showActual: false, actualUsage: null })

const { t } = useI18n()
const labels = useGPTAutoLabels()

const adjusted = computed(() => props.decision.status !== 'accepted')
const economics = computed(() => props.decision.economics)
const identity = (key: string) => key

const distributions = computed(() => [
  { key: 'model', title: t('gptAuto.detail.modelProbabilities'), rows: sortedProbabilities(props.classification.model_probabilities), label: identity },
  { key: 'effort', title: t('gptAuto.detail.effortProbabilities'), rows: sortedProbabilities(props.classification.effort_probabilities), label: labels.effort },
  { key: 'complexity', title: t('gptAuto.detail.complexityProbabilities'), rows: sortedProbabilities(props.classification.complexity_probabilities), label: labels.complexity },
  { key: 'category', title: t('gptAuto.detail.categoryProbabilities'), rows: sortedProbabilities(props.classification.category_probabilities), label: labels.category },
])
</script>
