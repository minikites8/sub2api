<template>
  <section class="usage-panel usage-model-chart">
    <div class="usage-panel__header">
      <div>
        <p class="usage-panel__eyebrow">CREDIT ALLOCATION</p>
        <h2>{{ t('usage.analytics.creditByModel') }}</h2>
      </div>
    </div>

    <div v-if="loading" class="usage-model-chart__state"><LoadingSpinner /></div>
    <div v-else-if="chartData" class="usage-model-chart__content">
      <div class="usage-model-chart__donut">
        <Doughnut :data="chartData" :options="chartOptions" />
        <div class="usage-model-chart__total">
          <strong>{{ formatCompact(totalCredits) }}</strong>
          <span>CREDITS</span>
        </div>
      </div>
      <div class="usage-model-chart__legend">
        <div v-for="(model, index) in displayModels" :key="model.model" class="usage-model-chart__row">
          <span class="usage-model-chart__swatch" :style="{ backgroundColor: palette[index] }" />
          <span class="usage-model-chart__name" :title="model.model">{{ model.model }}</span>
          <strong>{{ formatCredits(usdToCredits(model.actual_cost)) }}</strong>
          <span>{{ getPercentage(model.actual_cost) }}%</span>
        </div>
      </div>
    </div>
    <div v-else class="usage-model-chart__state">{{ t('usage.analytics.noChartData') }}</div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArcElement, Chart as ChartJS, Tooltip } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatCredits, usdToCredits } from '@/utils/credit'
import type { ModelStat } from '@/types'

ChartJS.register(ArcElement, Tooltip)

const props = withDefaults(defineProps<{
  modelStats: ModelStat[]
  loading?: boolean
}>(), {
  loading: false,
})

const { t } = useI18n()
const palette = ['#00f5a8', '#4f8cff', '#ffbd59', '#ff8d85', '#a98bff', '#4bd5e7']

const displayModels = computed(() => [...props.modelStats]
  .filter((item) => item.actual_cost > 0)
  .sort((a, b) => b.actual_cost - a.actual_cost)
  .slice(0, 6))

const totalUsd = computed(() => displayModels.value.reduce((sum, item) => sum + item.actual_cost, 0))
const totalCredits = computed(() => usdToCredits(totalUsd.value))

const chartData = computed(() => {
  if (!displayModels.value.length) return null
  return {
    labels: displayModels.value.map((item) => item.model),
    datasets: [{
      data: displayModels.value.map((item) => usdToCredits(item.actual_cost)),
      backgroundColor: displayModels.value.map((_, index) => palette[index]),
      borderColor: '#111a1e',
      borderWidth: 3,
      hoverOffset: 3,
    }],
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  cutout: '72%',
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: '#050a0d',
      borderColor: '#30443f',
      borderWidth: 1,
      titleColor: '#f2f7f5',
      bodyColor: '#a9bab5',
      callbacks: {
        label: (context: { label: string; raw: unknown }) =>
          `${context.label}: ${formatCredits(Number(context.raw))} Credits`,
      },
    },
  },
}

const formatCompact = (value: number) => new Intl.NumberFormat(undefined, {
  notation: value >= 10_000 ? 'compact' : 'standard',
  maximumFractionDigits: 1,
}).format(value)

const getPercentage = (value: number) => totalUsd.value > 0
  ? ((value / totalUsd.value) * 100).toFixed(0)
  : '0'
</script>

<style scoped>
.usage-model-chart { min-height: 390px; }
.usage-model-chart__content { display: flex; min-height: 300px; flex-direction: column; align-items: center; gap: 18px; padding: 4px 18px 18px; }
.usage-model-chart__donut { position: relative; width: 190px; height: 190px; flex: none; }
.usage-model-chart__total { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; pointer-events: none; }
.usage-model-chart__total strong { color: #f2f7f5; font-size: 20px; line-height: 1.1; }
.usage-model-chart__total span { margin-top: 5px; color: #73867f; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 9px; letter-spacing: 0; }
.usage-model-chart__legend { display: grid; width: 100%; gap: 7px; }
.usage-model-chart__row { display: grid; grid-template-columns: 9px minmax(0, 1fr) auto 34px; align-items: center; gap: 8px; border: 1px solid rgba(72, 96, 89, .55); background: rgba(4, 10, 13, .45); padding: 7px 9px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 10px; }
.usage-model-chart__swatch { width: 8px; height: 8px; border-radius: 50%; }
.usage-model-chart__name { overflow: hidden; color: #c6d2ce; text-overflow: ellipsis; white-space: nowrap; }
.usage-model-chart__row strong { color: #edf5f2; font-weight: 600; }
.usage-model-chart__row > span:last-child { color: #71847d; text-align: right; }
.usage-model-chart__state { display: flex; height: 300px; align-items: center; justify-content: center; color: #83958f; font-size: 13px; }
</style>
