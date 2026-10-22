<template>
  <div class="space-y-6" data-testid="gpt-auto-overview">
    <div class="card flex flex-wrap items-end gap-3 p-4">
      <label class="text-sm">
        <span class="input-label">{{ t('gptAuto.overview.range') }}</span>
        <select v-model="range" class="input" data-testid="gpt-auto-overview-range" @change="load">
          <option v-for="option in rangeOptions" :key="option" :value="option">{{ t(`gptAuto.overview.ranges.${option}`) }}</option>
        </select>
      </label>
      <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        <span>{{ t('common.refresh') }}</span>
      </button>
      <p v-if="overview" class="ml-auto text-xs text-gray-500 dark:text-gray-400">
        {{ t('gptAuto.overview.rangeHint', { start: formatDateTime(overview.start_time), end: formatDateTime(overview.end_time) }) }}
      </p>
    </div>

    <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>

    <div v-if="loading && !overview" class="flex justify-center py-12">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <template v-else-if="overview">
      <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
        <div v-for="tile in tiles" :key="tile.key" class="card p-4" :data-testid="`gpt-auto-tile-${tile.key}`">
          <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ tile.label }}</p>
          <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ tile.value }}</p>
          <p v-if="tile.meta" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ tile.meta }}</p>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.overview.runtimeTitle') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.runtimeHint') }}</span>
        </div>
        <div class="grid grid-cols-2 gap-3 text-sm md:grid-cols-4 xl:grid-cols-6">
          <div v-for="(value, key) in overview.runtime.counters" :key="key">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ labels.counter(String(key)) }}</p>
            <p class="font-medium text-gray-900 dark:text-white">{{ formatNumber(value) }}</p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.queue') }}</p>
            <p class="font-medium text-gray-900 dark:text-white">{{ overview.runtime.queue_length }} / {{ overview.runtime.queue_capacity }}</p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.circuit') }}</p>
            <p class="font-medium" :class="overview.runtime.circuit_state === 'closed' ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
              {{ t(`gptAuto.overview.circuitStates.${overview.runtime.circuit_state}`) }}
            </p>
          </div>
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.todayBudget') }}</p>
            <p class="font-medium text-gray-900 dark:text-white" data-testid="gpt-auto-today-budget">{{ formatNumber(overview.runtime.today_calls) }} · {{ formatUSD(overview.runtime.today_cost) }}</p>
          </div>
        </div>
        <p v-if="overview.runtime.budget_scope === 'unavailable'" class="mt-3 rounded bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="gpt-auto-budget-unavailable">
          {{ t('gptAuto.overview.budgetUnavailable') }}
        </p>
        <div v-if="clusterCounters.length" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700" data-testid="gpt-auto-cluster-counters">
          <p class="mb-2 text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('gptAuto.overview.clusterTitle') }}</p>
          <div class="grid grid-cols-2 gap-3 text-sm md:grid-cols-4 xl:grid-cols-6">
            <div v-for="[key, value] in clusterCounters" :key="key">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ labels.counter(key) }}</p>
              <p class="font-medium text-gray-900 dark:text-white">{{ formatNumber(value) }}</p>
            </div>
          </div>
        </div>
        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ t('gptAuto.overview.scopeNote') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('gptAuto.overview.excludedEndpoints', { list: overview.runtime.excluded_endpoints.join(', ') }) }}
        </p>
      </div>

      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">
        {{ t('gptAuto.overview.hypotheticalNotice') }}
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.finalModels')" :badge="t('gptAuto.common.hypothetical')" :items="overview.stored.final_models" :empty-text="t('gptAuto.common.noData')" :color-index="1" test-id="gpt-auto-chart-final-models" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.jevModels')" :badge="t('gptAuto.common.hypothetical')" :items="overview.stored.jev_models" :empty-text="t('gptAuto.common.noData')" :color-index="2" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.efforts')" :badge="t('gptAuto.common.hypothetical')" :items="overview.stored.final_efforts" :label-for="labels.effort" :empty-text="t('gptAuto.common.noData')" :color-index="3" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.jevEfforts')" :badge="t('gptAuto.common.hypothetical')" :items="overview.stored.jev_efforts ?? []" :label-for="labels.effort" :empty-text="t('gptAuto.common.noData')" :color-index="4" test-id="gpt-auto-chart-jev-efforts" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.complexity')" :items="sortedComplexity" :label-for="labels.complexity" :empty-text="t('gptAuto.common.noData')" :color-index="4" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.categories')" :items="overview.stored.categories" :label-for="labels.category" :empty-text="t('gptAuto.common.noData')" :color-index="5" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.errors')" :items="overview.stored.error_classes" :label-for="labels.errorClass" :empty-text="t('gptAuto.common.noData')" :color-index="2" />
        <GPTAutoBarChart :title="t('gptAuto.overview.charts.classifierConfigs')" :items="overview.stored.classifier_configs ?? []" :label-for="classifierLabel" :empty-text="t('gptAuto.common.noData')" :color-index="1" test-id="gpt-auto-chart-classifier-configs" />
      </div>

      <div class="card p-4" data-testid="gpt-auto-trend">
        <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('gptAuto.overview.charts.trend') }} · {{ t(`gptAuto.overview.buckets.${overview.stored.trend_bucket_size || 'hour'}`) }}
        </h3>
        <div v-if="overview.stored.trend.length > 0" class="h-56">
          <Line :data="trendData" :options="trendOptions" />
        </div>
        <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.common.noData') }}</p>
      </div>

      <div class="card overflow-hidden" data-testid="gpt-auto-actual-vs-final">
        <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.overview.matrixTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.matrixHint') }}</p>
        </div>
        <div v-if="matrix.rows.length > 0" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.overview.matrixActual') }}</th>
                <th v-for="column in matrix.columns" :key="column" class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400">{{ column || '—' }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in matrix.rows" :key="row">
                <td class="px-4 py-2 font-medium text-gray-900 dark:text-white">{{ row || '—' }}</td>
                <td v-for="column in matrix.columns" :key="column" class="px-4 py-2 text-right tabular-nums" :class="row === column ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-700 dark:text-gray-300'">
                  {{ matrix.cells[`${row}|${column}`] ?? '' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.common.noData') }}</p>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Line } from 'vue-chartjs'
import { CategoryScale, Chart as ChartJS, Filler, Legend, LinearScale, LineElement, PointElement, Tooltip } from 'chart.js'
import Icon from '@/components/icons/Icon.vue'
import GPTAutoBarChart from './GPTAutoBarChart.vue'
import { gptAutoAPI, type GPTAutoOverview } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { formatMs, formatNumber, formatPercent, formatUSD } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'
import { useThemeRevision } from '@/composables/useThemeRevision'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const { t } = useI18n()
const labels = useGPTAutoLabels()

type RangeKey = '1h' | '24h' | '7d' | '30d'
const rangeOptions: RangeKey[] = ['1h', '24h', '7d', '30d']
const rangeHours: Record<RangeKey, number> = { '1h': 1, '24h': 24, '7d': 168, '30d': 720 }
const range = ref<RangeKey>('24h')
const overview = ref<GPTAutoOverview | null>(null)
const loading = ref(false)
const error = ref('')
let sequence = 0
let timer: number | undefined

async function load() {
  const current = ++sequence
  loading.value = true
  error.value = ''
  const end = new Date()
  const start = new Date(end.getTime() - rangeHours[range.value] * 3600 * 1000)
  try {
    const result = await gptAutoAPI.getOverview({ start_time: start.toISOString(), end_time: end.toISOString() })
    if (current === sequence) overview.value = result
  } catch (err) {
    if (current === sequence) error.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    if (current === sequence) loading.value = false
  }
}

const tiles = computed(() => {
  const value = overview.value
  if (!value) return []
  const stored = value.stored
  const runtime = value.runtime
  const cluster = runtime.cluster_counters_today
  const counters = cluster ?? runtime.counters
  const skipped = ['skipped_filter', 'skipped_budget', 'skipped_budget_unavailable', 'skipped_circuit_open', 'skipped_not_configured', 'skipped_no_text']
    .reduce((sum, key) => sum + (counters[key] ?? 0), 0)
  return [
    { key: 'status', label: t('gptAuto.overview.tiles.status'), value: value.shadow_enabled ? t('gptAuto.common.enabled') : t('gptAuto.common.disabled'), meta: t('gptAuto.overview.tiles.statusMeta', { rate: value.sample_rate, version: value.policy_version }) },
    { key: 'sampled', label: t('gptAuto.overview.tiles.sampled'), value: formatNumber(stored.total), meta: t('gptAuto.overview.tiles.sampledMeta', { success: formatNumber(stored.success), failed: formatNumber(stored.failed) }) },
    { key: 'errorRate', label: t('gptAuto.overview.tiles.errorRate'), value: formatPercent(value.error_rate), meta: t('gptAuto.overview.tiles.errorRateMeta', { adjusted: formatNumber(stored.adjusted), noEligible: formatNumber(stored.no_eligible) }) },
    { key: 'latency', label: t('gptAuto.overview.tiles.latency'), value: formatMs(stored.avg_latency_ms), meta: stored.p50_latency_ms !== undefined && stored.p50_latency_ms !== null ? t('gptAuto.overview.tiles.latencyMeta', { p50: formatMs(stored.p50_latency_ms), p95: formatMs(stored.p95_latency_ms) }) : t('gptAuto.overview.tiles.latencyTooFew') },
    { key: 'tokens', label: t('gptAuto.overview.tiles.tokens'), value: formatNumber(stored.input_tokens + stored.output_tokens), meta: t('gptAuto.overview.tiles.tokensMeta', { input: formatNumber(stored.input_tokens), output: formatNumber(stored.output_tokens) }) },
    { key: 'cost', label: t('gptAuto.overview.tiles.cost'), value: formatUSD(stored.classifier_cost), meta: t('gptAuto.overview.tiles.costMeta') },
    { key: 'skipped', label: cluster ? t('gptAuto.overview.tiles.skippedCluster') : t('gptAuto.overview.tiles.skipped'), value: formatNumber(skipped), meta: t('gptAuto.overview.tiles.skippedMeta', { dropped: formatNumber(counters.dropped_queue_full ?? 0) }) },
    { key: 'policyChanges', label: t('gptAuto.overview.tiles.policyChanges'), value: formatNumber(stored.policy_model_changes ?? 0), meta: t('gptAuto.overview.tiles.policyChangesMeta', { effort: formatNumber(stored.policy_effort_changes ?? 0) }) },
    { key: 'belowTarget', label: t('gptAuto.overview.tiles.belowTarget'), value: `${formatNumber(stored.model_below_target_count ?? 0)} / ${formatNumber(stored.below_target_count)}`, meta: t('gptAuto.overview.tiles.belowTargetMeta', { insufficient: formatNumber(stored.insufficient_estimates ?? 0) }) },
  ]
})

const clusterCounters = computed(() =>
  Object.entries(overview.value?.runtime.cluster_counters_today ?? {}).sort(([a], [b]) => a.localeCompare(b)),
)

const classifierLabel = (key: string) => (key ? key.slice(0, 10) : t('gptAuto.detail.classifierLegacy'))

const sortedComplexity = computed(() =>
  [...(overview.value?.stored.complexity ?? [])].sort((a, b) => Number(a.key) - Number(b.key)),
)

const matrix = computed(() => {
  const pairs = overview.value?.stored.actual_vs_final ?? []
  const rows = [...new Set(pairs.map((pair) => pair.actual))].sort()
  const columns = [...new Set(pairs.map((pair) => pair.recommended))].sort()
  const cells: Record<string, number> = {}
  for (const pair of pairs) cells[`${pair.actual}|${pair.recommended}`] = pair.count
  return { rows, columns, cells }
})

const themeRevision = useThemeRevision()
const cssVar = (name: string) => {
  void themeRevision.value
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

const trendData = computed(() => {
  const points = overview.value?.stored.trend ?? []
  const daily = overview.value?.stored.trend_bucket_size === 'day'
  return {
    labels: points.map((point) => {
      const date = new Date(point.bucket)
      return daily ? date.toLocaleDateString() : date.toLocaleString([], { month: 'numeric', day: 'numeric', hour: '2-digit' })
    }),
    datasets: [
      { label: t('gptAuto.evaluationStatus.success'), data: points.map((point) => point.success), borderColor: cssVar('--md-chart-1') || '#6366f1', backgroundColor: 'transparent', tension: 0.3 },
      { label: t('gptAuto.evaluationStatus.failed'), data: points.map((point) => point.failed), borderColor: cssVar('--md-chart-2') || '#ef4444', backgroundColor: 'transparent', tension: 0.3 },
    ],
  }
})

const trendOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { labels: { color: cssVar('--md-on-surface-variant') } } },
  scales: {
    x: { ticks: { color: cssVar('--md-on-surface-variant') }, grid: { color: cssVar('--md-outline-variant') } },
    y: { beginAtZero: true, ticks: { precision: 0, color: cssVar('--md-on-surface-variant') }, grid: { color: cssVar('--md-outline-variant') } },
  },
}))

onMounted(() => {
  void load()
  timer = window.setInterval(() => { void load() }, 30000)
})
onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer)
})

defineExpose({ load })
</script>
