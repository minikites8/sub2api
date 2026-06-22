<template>
  <section class="md3-charts-shell">
    <div class="md3-chart-toolbar">
      <div class="md3-toolbar-group md3-toolbar-date">
        <span>{{ t('dashboard.timeRange') }}</span>
        <DateRangePicker
          :start-date="startDate"
          :end-date="endDate"
          @update:startDate="$emit('update:startDate', $event)"
          @update:endDate="$emit('update:endDate', $event)"
          @change="$emit('dateRangeChange', $event)"
        />
      </div>

      <button type="button" class="md3-refresh-button" :disabled="loading" @click="$emit('refresh')">
        <Icon name="refresh" size="sm" />
        <span>{{ t('common.refresh') }}</span>
      </button>

      <div class="md3-toolbar-group md3-toolbar-granularity">
        <span>{{ t('dashboard.granularity') }}</span>
        <Select
          :model-value="granularity"
          :options="granularityOptions"
          size="sm"
          @update:model-value="$emit('update:granularity', $event)"
          @change="$emit('granularityChange')"
        />
      </div>
    </div>

    <div class="md3-chart-grid">
      <article class="md3-model-card">
        <div v-if="loading" class="md3-card-loading">
          <LoadingSpinner size="md" />
        </div>
        <header class="md3-card-header">
          <h2>{{ t('dashboard.modelDistribution') }}</h2>
        </header>

        <div class="md3-model-content">
          <div class="md3-doughnut-frame">
            <Doughnut v-if="modelData" :data="modelData" :options="doughnutOptions" />
            <div v-else class="md3-empty-chart">{{ t('dashboard.noDataAvailable') }}</div>
          </div>

          <div class="md3-model-table-wrap">
            <table class="md3-model-table">
              <thead>
                <tr>
                  <th>{{ t('dashboard.model') }}</th>
                  <th>{{ t('dashboard.requests') }}</th>
                  <th>{{ t('dashboard.tokens') }}</th>
                  <th>{{ t('dashboard.actual') }}</th>
                  <th>{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="model in models" :key="model.model">
                  <td :title="model.model">
                    <span>{{ model.model }}</span>
                  </td>
                  <td>{{ formatNumber(model.requests) }}</td>
                  <td>{{ formatTokens(model.total_tokens) }}</td>
                  <td class="md3-actual-cost">${{ formatCost(model.actual_cost) }}</td>
                  <td class="md3-standard-cost">${{ formatCost(model.cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </article>

      <div class="md3-trend-card">
        <TokenUsageTrend :trend-data="trend" :loading="loading" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import { Doughnut } from 'vue-chartjs'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler } from 'chart.js'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler)

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()

const granularityOptions = computed(() => [
  { value: 'day', label: t('dashboard.day') },
  { value: 'hour', label: t('dashboard.hour') }
])

const modelData = computed(() => !props.models?.length ? null : {
  labels: props.models.map((m: ModelStat) => m.model),
  datasets: [{
    data: props.models.map((m: ModelStat) => m.total_tokens),
    backgroundColor: ['#f1f1f1', '#aaaaaa', '#717171', '#3f3f3f', '#272727', '#606060', '#909090', '#cccccc'],
    borderWidth: 0
  }]
})

const doughnutOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.label}: ${formatTokens(context.parsed)} tokens`
      }
    }
  }
}
</script>

<style scoped>
.md3-charts-shell {
  display: grid;
  gap: 16px;
}

.md3-chart-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  padding: 12px;
  box-shadow: var(--md-elevation-1);
}

.dark .md3-chart-toolbar {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-toolbar-group {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.md3-toolbar-group > span {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 800;
  white-space: nowrap;
}

.md3-toolbar-date {
  flex: 1 1 320px;
}

.md3-toolbar-granularity {
  margin-left: auto;
}

.md3-toolbar-granularity :deep(.relative) {
  width: 118px;
}

.md3-refresh-button {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: 999px;
  background: var(--md-primary-container);
  padding: 0 12px;
  color: var(--md-on-primary-container);
  font-size: 0.8125rem;
  font-weight: 800;
  transition: background-color 160ms ease, border-color 160ms ease;
}

.md3-refresh-button:hover:not(:disabled) {
  background: var(--md-surface-container-high);
}

.md3-refresh-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.dark .md3-toolbar-group > span {
  color: var(--md-on-surface-variant);
}

.dark .md3-refresh-button {
  background: var(--md-primary-container);
  color: var(--md-on-primary-container);
}

.dark .md3-refresh-button:hover:not(:disabled) {
  background: var(--md-surface-container-high);
}

.md3-chart-toolbar :deep(.date-picker-trigger),
.md3-chart-toolbar :deep(.select-trigger) {
  min-height: 34px;
  border-radius: 999px;
  background: var(--md-surface-container);
  padding-top: 0.375rem;
  padding-bottom: 0.375rem;
  box-shadow: none;
}

.dark .md3-chart-toolbar :deep(.date-picker-trigger),
.dark .md3-chart-toolbar :deep(.select-trigger) {
  background: var(--md-surface-container-low);
}

.md3-chart-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 0.95fr);
  gap: 16px;
}

.md3-model-card,
.md3-trend-card :deep(.card) {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-model-card {
  position: relative;
  overflow: hidden;
  padding: 16px;
}

.md3-trend-card :deep(.card) {
  height: 100%;
}

.dark .md3-model-card,
.dark .md3-trend-card :deep(.card) {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-card-loading {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--md-surface) 84%, transparent);
}

.dark .md3-card-loading {
  background: color-mix(in srgb, var(--md-surface) 84%, transparent);
}

.md3-card-header {
  margin-bottom: 14px;
}

.md3-card-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  font-weight: 760;
}

.dark .md3-card-header h2 {
  color: var(--md-on-surface);
}

.md3-model-content {
  display: grid;
  grid-template-columns: 184px minmax(0, 1fr);
  gap: 18px;
  align-items: center;
}

.md3-doughnut-frame {
  width: 184px;
  height: 184px;
}

.md3-empty-chart {
  display: flex;
  height: 100%;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--md-outline-variant);
  border-radius: 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
  text-align: center;
}

.dark .md3-empty-chart {
  border-color: var(--md-outline-variant);
  color: var(--md-on-surface-variant);
}

.md3-model-table-wrap {
  max-height: 210px;
  min-width: 0;
  overflow: auto;
}

.md3-model-table {
  width: 100%;
  min-width: 460px;
  border-collapse: collapse;
  font-size: 0.75rem;
}

.md3-model-table th {
  padding: 0 0 8px 10px;
  color: var(--md-on-surface-variant);
  font-weight: 800;
  text-align: right;
  white-space: nowrap;
}

.md3-model-table th:first-child {
  padding-left: 0;
  text-align: left;
}

.md3-model-table td {
  border-top: 1px solid var(--md-outline-variant);
  padding: 8px 0 8px 10px;
  color: var(--md-on-surface-variant);
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}

.md3-model-table td:first-child {
  max-width: 160px;
  padding-left: 0;
  color: var(--md-on-surface);
  font-weight: 700;
  text-align: left;
}

.md3-model-table td:first-child span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-model-table .md3-actual-cost {
  color: var(--md-on-surface);
  font-weight: 800;
}

.md3-model-table .md3-standard-cost {
  color: color-mix(in srgb, var(--md-on-surface-variant) 70%, transparent);
}

.dark .md3-model-table th {
  color: var(--md-on-surface-variant);
}

.dark .md3-model-table td {
  border-color: var(--md-outline-variant);
  color: var(--md-on-surface-variant);
}

.dark .md3-model-table td:first-child {
  color: var(--md-on-surface);
}

.dark .md3-model-table .md3-actual-cost {
  color: var(--md-on-surface);
}

.dark .md3-model-table .md3-standard-cost {
  color: color-mix(in srgb, var(--md-on-surface-variant) 64%, transparent);
}

@media (max-width: 1180px) {
  .md3-chart-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 720px) {
  .md3-chart-toolbar {
    align-items: stretch;
  }

  .md3-toolbar-group {
    width: 100%;
    align-items: flex-start;
    flex-direction: column;
  }

  .md3-toolbar-date,
  .md3-toolbar-granularity {
    flex: 1 1 auto;
    margin-left: 0;
  }

  .md3-toolbar-date :deep(.relative),
  .md3-toolbar-date :deep(.date-picker-trigger),
  .md3-toolbar-granularity :deep(.relative),
  .md3-refresh-button {
    width: 100%;
  }

  .md3-model-content {
    grid-template-columns: minmax(0, 1fr);
    justify-items: center;
  }

  .md3-model-table-wrap {
    width: 100%;
  }
}
</style>
