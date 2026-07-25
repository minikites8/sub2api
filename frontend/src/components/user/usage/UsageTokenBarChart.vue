<template>
  <section class="usage-panel usage-token-chart">
    <div class="usage-panel__header">
      <div>
        <p class="usage-panel__eyebrow">TOKEN FLOW</p>
        <h2>{{ t('usage.analytics.dailyTokenUsage') }}</h2>
      </div>
      <div class="usage-chart-legend" aria-hidden="true">
        <span><i class="usage-chart-legend__dot usage-chart-legend__dot--input" />{{ t('usage.analytics.inputTokens') }}</span>
        <span><i class="usage-chart-legend__dot usage-chart-legend__dot--output" />{{ t('usage.analytics.outputTokens') }}</span>
      </div>
    </div>

    <div v-if="loading" class="usage-chart-state">
      <LoadingSpinner />
    </div>
    <div v-else-if="chartData" class="usage-chart-canvas">
      <Bar :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="usage-chart-state">{{ t('usage.analytics.noChartData') }}</div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { TrendDataPoint } from '@/types'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip, Legend)

const props = withDefaults(defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
}>(), {
  loading: false,
})

const { t } = useI18n()

const compactNumber = (value: number) => new Intl.NumberFormat(undefined, {
  notation: 'compact',
  maximumFractionDigits: 1,
}).format(value)

const formatDate = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric' }).format(date)
}

const chartData = computed(() => {
  if (!props.trendData.length) return null
  return {
    labels: props.trendData.map((item) => formatDate(item.date)),
    datasets: [
      {
        label: t('usage.analytics.inputTokens'),
        data: props.trendData.map((item) => item.input_tokens + item.cache_creation_tokens + item.cache_read_tokens),
        backgroundColor: '#263a39',
        hoverBackgroundColor: '#36514d',
        borderColor: '#47615c',
        borderWidth: 1,
        borderRadius: 2,
        borderSkipped: false,
        stack: 'tokens',
      },
      {
        label: t('usage.analytics.outputTokens'),
        data: props.trendData.map((item) => item.output_tokens),
        backgroundColor: '#00f5a8',
        hoverBackgroundColor: '#41ffc2',
        borderColor: '#00f5a8',
        borderWidth: 1,
        borderRadius: 2,
        borderSkipped: false,
        stack: 'tokens',
      },
    ],
  }
})

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: '#050a0d',
      borderColor: '#30443f',
      borderWidth: 1,
      titleColor: '#f2f7f5',
      bodyColor: '#a9bab5',
      padding: 12,
      callbacks: {
        label: (context: { dataset: { label?: string }; raw: unknown }) =>
          `${context.dataset.label}: ${Number(context.raw).toLocaleString()}`,
      },
    },
  },
  scales: {
    x: {
      stacked: true,
      border: { color: '#30443f' },
      grid: { display: false },
      ticks: { color: '#83958f', font: { family: 'ui-monospace, SFMono-Regular, Menlo, monospace', size: 11 } },
    },
    y: {
      stacked: true,
      beginAtZero: true,
      border: { display: false },
      grid: { color: 'rgba(72, 96, 89, 0.28)' },
      ticks: {
        color: '#83958f',
        padding: 10,
        font: { family: 'ui-monospace, SFMono-Regular, Menlo, monospace', size: 11 },
        callback: (value: string | number) => compactNumber(Number(value)),
      },
    },
  },
}))
</script>

<style scoped>
.usage-token-chart { min-height: 390px; }
.usage-chart-canvas { height: 300px; padding: 8px 18px 18px; }
.usage-chart-state { display: flex; height: 300px; align-items: center; justify-content: center; color: #83958f; font-size: 13px; }
.usage-chart-legend { display: flex; flex-wrap: wrap; gap: 16px; color: #91a39d; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.usage-chart-legend span { display: inline-flex; align-items: center; gap: 7px; }
.usage-chart-legend__dot { width: 9px; height: 9px; border-radius: 2px; background: #263a39; border: 1px solid #47615c; }
.usage-chart-legend__dot--output { background: #00f5a8; border-color: #00f5a8; }
@media (max-width: 640px) {
  .usage-token-chart { min-height: 340px; }
  .usage-chart-canvas, .usage-chart-state { height: 250px; }
  .usage-chart-legend { display: none; }
}
</style>
