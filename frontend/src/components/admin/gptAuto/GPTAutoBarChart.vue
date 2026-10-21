<template>
  <div class="card p-4" :data-testid="testId">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h3>
      <span v-if="badge" class="rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ badge }}</span>
    </div>
    <div v-if="items.length > 0" :style="{ height: `${Math.max(120, items.length * 30 + 30)}px` }">
      <Bar :data="chartData" :options="chartOptions" />
    </div>
    <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ emptyText }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Bar } from 'vue-chartjs'
import { BarElement, CategoryScale, Chart as ChartJS, LinearScale, Tooltip } from 'chart.js'
import type { GPTAutoCount } from '@/api/admin/gptAuto'
import { useThemeRevision } from '@/composables/useThemeRevision'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip)

const props = withDefaults(defineProps<{
  title: string
  items: GPTAutoCount[]
  emptyText: string
  labelFor?: (key: string) => string
  badge?: string
  colorIndex?: number
  testId?: string
}>(), {
  labelFor: (key: string) => key || '—',
  badge: '',
  colorIndex: 1,
  testId: undefined,
})

const themeRevision = useThemeRevision()
const cssVar = (name: string) => {
  void themeRevision.value
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

const chartData = computed(() => ({
  labels: props.items.map((item) => props.labelFor(item.key)),
  datasets: [{
    data: props.items.map((item) => item.count),
    backgroundColor: cssVar(`--md-chart-${props.colorIndex}`) || '#6366f1',
    borderRadius: 4,
    maxBarThickness: 18,
  }],
}))

const chartOptions = computed(() => ({
  indexAxis: 'y' as const,
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } },
  scales: {
    x: { beginAtZero: true, ticks: { precision: 0, color: cssVar('--md-on-surface-variant') }, grid: { color: cssVar('--md-outline-variant') } },
    y: { ticks: { color: cssVar('--md-on-surface-variant') }, grid: { display: false } },
  },
}))
</script>
