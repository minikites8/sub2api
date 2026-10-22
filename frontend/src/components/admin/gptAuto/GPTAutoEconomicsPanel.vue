<template>
  <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" :data-testid="testId">
    <div class="flex flex-wrap items-center gap-2">
      <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ title ?? t('gptAuto.detail.economics') }}</p>
      <span class="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('gptAuto.common.hypothetical') }}</span>
      <span class="rounded-full px-2 py-0.5 text-[11px]" :class="qualityClass" data-testid="gpt-auto-estimate-quality">{{ labels.estimateQuality(view.quality) }}</span>
    </div>
    <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">{{ t('gptAuto.economics.notObserved') }}</p>

    <p v-if="!economics.pricing_available" class="mt-2 text-xs text-amber-600">{{ t('gptAuto.simulator.noPricing') }}</p>
    <template v-else>
      <p v-if="view.quality === 'insufficient_data'" class="mt-2 rounded bg-gray-50 px-2 py-1.5 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300" data-testid="gpt-auto-estimate-insufficient">
        {{ t('gptAuto.economics.insufficient') }}
      </p>
      <p v-else-if="view.quality === 'heuristic'" class="mt-2 rounded bg-amber-50 px-2 py-1.5 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" data-testid="gpt-auto-estimate-heuristic">
        {{ t('gptAuto.economics.heuristic') }}
      </p>
      <p v-else-if="view.legacy" class="mt-2 text-xs text-gray-500">{{ t('gptAuto.economics.legacy') }}</p>

      <div class="mt-3 grid grid-cols-1 gap-2 md:grid-cols-3">
        <div class="rounded border border-gray-100 p-2 dark:border-dark-700" data-testid="gpt-auto-model-margin">
          <p class="text-[11px] font-medium uppercase tracking-wide text-gray-500">{{ t('gptAuto.economics.modelMargin') }}</p>
          <p class="text-lg font-semibold tabular-nums" :class="marginStatusClass(view.modelStatus)">{{ marginText(view.modelMargin) }}</p>
          <p class="text-[11px]" :class="marginStatusClass(view.modelStatus)">{{ labels.marginStatus(view.modelStatus) }} · {{ t('gptAuto.economics.target', { value: formatPercent(economics.target_margin) }) }}</p>
          <p class="mt-1 text-[11px] text-gray-500">{{ t('gptAuto.economics.modelMarginHint', { revenue: money(economics.estimated_revenue), cost: money(economics.estimated_upstream_cost) }) }}</p>
        </div>
        <div class="rounded border border-gray-100 p-2 dark:border-dark-700" data-testid="gpt-auto-full-margin">
          <p class="text-[11px] font-medium uppercase tracking-wide text-gray-500">{{ t('gptAuto.economics.fullMargin') }}</p>
          <p class="text-lg font-semibold tabular-nums" :class="marginStatusClass(view.fullStatus)">{{ marginText(view.fullMargin) }}</p>
          <p class="text-[11px]" :class="marginStatusClass(view.fullStatus)">{{ labels.marginStatus(view.fullStatus) }}</p>
          <p class="mt-1 text-[11px] text-gray-500">{{ t('gptAuto.economics.fullMarginHint', { profit: money(economics.estimated_gross_profit) }) }}</p>
        </div>
        <div class="rounded border border-gray-100 p-2 dark:border-dark-700" data-testid="gpt-auto-classifier-overhead">
          <p class="text-[11px] font-medium uppercase tracking-wide text-gray-500">{{ t('gptAuto.economics.overhead') }}</p>
          <p class="text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatUSD(economics.estimated_classifier_cost) }}</p>
          <p class="text-[11px] text-gray-500">{{ labels.classifierCostSource(economics.classifier_cost_source ?? 'actual_jev_call') }}</p>
          <p class="mt-1 text-[11px] text-gray-500">{{ t('gptAuto.economics.overheadHint') }}</p>
        </div>
      </div>

      <dl class="mt-3 grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.inputTokens') }}</dt><dd class="tabular-nums">{{ formatNumber(economics.estimated_input_tokens) }} <span class="block text-[11px] text-gray-500">{{ labels.estimateSource(economics.input_estimate_source ?? 'heuristic_char_count') }}</span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.cachedTokens') }}</dt><dd class="tabular-nums">{{ formatNumber(economics.estimated_cached_input_tokens ?? 0) }} <span class="block text-[11px] text-gray-500">{{ labels.estimateSource(economics.cached_input_estimate_source ?? 'assumed_zero') }}</span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.outputTokens') }}</dt><dd class="tabular-nums">{{ economics.output_estimate_source === 'insufficient_data' || economics.output_estimate_source === 'no_history' ? 'N/A' : formatNumber(Math.round(economics.estimated_output_tokens)) }} <span class="block text-[11px] text-gray-500">{{ labels.outputSource(economics.output_estimate_source) }}<template v-if="economics.output_estimate_samples"> · n={{ economics.output_estimate_samples }}</template></span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.multipliers') }}</dt><dd class="tabular-nums">{{ formatMultiplier(economics.billing_multiplier) }} · {{ formatMultiplier(economics.effort_billing_multiplier ?? 1) }} · {{ formatMultiplier(economics.customer_rate_multiplier ?? 1) }}<span class="block text-[11px] text-gray-500">{{ labels.rateSource(economics.customer_rate_source ?? 'list_rate_assumed') }}</span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.listPrice') }}</dt><dd class="tabular-nums">{{ formatRate(economics.list_input_per_m) }} / {{ formatRate(economics.list_cached_input_per_m ?? 0) }} / {{ formatRate(economics.list_output_per_m) }} {{ t('gptAuto.detail.perM') }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.detail.upstreamPrice') }}</dt><dd class="tabular-nums">{{ formatRate(economics.upstream_input_per_m) }} / {{ formatRate(economics.upstream_cached_input_per_m ?? economics.upstream_input_per_m) }} / {{ formatRate(economics.upstream_output_per_m) }} {{ t('gptAuto.detail.perM') }}<span class="block text-[11px] text-gray-500">{{ economics.upstream_channel || labels.costSource(economics.upstream_cost_source) }}</span></dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.requiredModel') }}</dt><dd class="tabular-nums">{{ formatMultiplier(economics.min_required_multiplier) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('gptAuto.economics.requiredFull') }}</dt><dd class="tabular-nums">{{ formatMultiplier(economics.min_required_multiplier_fully_loaded) }}</dd></div>
      </dl>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GPTAutoEconomics } from '@/api/admin/gptAuto'
import { economicsView, formatMultiplier, formatNumber, formatPercent, formatRate, formatUSD, marginStatusClass } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const props = withDefaults(defineProps<{ economics: GPTAutoEconomics; title?: string; testId?: string }>(), { title: undefined, testId: 'gpt-auto-economics' })

const { t } = useI18n()
const labels = useGPTAutoLabels()
const view = computed(() => economicsView(props.economics))
const unknown = computed(() => view.value.quality === 'insufficient_data')
const money = (value: number) => (unknown.value ? 'N/A' : formatUSD(value))
const marginText = (value: number | null) => (value === null ? 'N/A' : formatPercent(value))
const qualityClass = computed(() => {
  switch (view.value.quality) {
    case 'historical': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'admin_assumption': return 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
    case 'heuristic': return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
})
</script>
