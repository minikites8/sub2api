<template>
  <section class="md3-charts-shell">
    <article class="md3-trend-card">
      <header class="md3-card-header md3-chart-header">
        <div>
          <h2>{{ t('dashboard.tokenUsageTrend') }}</h2>
          <p>{{ t('dashboard.timeRange') }}</p>
        </div>
        <div class="md3-chart-controls">
          <DateRangePicker
            :start-date="startDate"
            :end-date="endDate"
            @update:startDate="$emit('update:startDate', $event)"
            @update:endDate="$emit('update:endDate', $event)"
            @change="$emit('dateRangeChange', $event)"
          />
          <Select
            :model-value="granularity"
            :options="granularityOptions"
            size="sm"
            @update:model-value="$emit('update:granularity', $event)"
            @change="$emit('granularityChange')"
          />
          <button
            type="button"
            class="md3-refresh-button"
            :disabled="loading"
            :title="t('common.refresh')"
            :aria-label="t('common.refresh')"
            @click="$emit('refresh')"
          >
            <Icon name="refresh" size="sm" />
          </button>
        </div>
      </header>
      <TokenUsageTrend :trend-data="trend" :loading="loading" embedded />
    </article>

    <article class="md3-model-card">
      <div class="md3-model-card-inner">
        <div v-if="loading" class="md3-card-loading">
          <LoadingSpinner size="md" />
        </div>
        <header class="md3-card-header">
          <div>
            <h2>{{ t('dashboard.modelDistribution') }}</h2>
            <p>{{ t('dashboard.requests') }} / {{ t('dashboard.tokens') }}</p>
          </div>
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
      </div>
    </article>

    <UserDashboardQuickActions class="md3-quick-actions-card" />
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
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
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
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.md3-trend-card,
.md3-model-card-inner {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-trend-card {
  display: grid;
  min-width: 0;
  grid-column: 1 / -1;
  gap: 8px;
  padding: 16px;
}

.md3-model-card {
  min-width: 0;
  grid-column: span 3;
}

.md3-quick-actions-card {
  min-width: 0;
  grid-column: span 1;
  align-self: stretch;
}

.md3-model-card-inner {
  position: relative;
  overflow: hidden;
  padding: 16px;
}

.dark .md3-trend-card,
.dark .md3-model-card-inner {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-chart-header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.md3-chart-controls {
  display: flex;
  flex: 1 1 420px;
  min-width: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.md3-chart-controls :deep(.relative) {
  min-width: 118px;
}

.md3-chart-controls :deep(.select-trigger) {
  width: 118px;
}

.md3-refresh-button {
  display: inline-flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, border-color 160ms ease;
}

.md3-refresh-button:hover:not(:disabled) {
  background: var(--md-state-hover);
  color: var(--md-on-surface);
}

.md3-refresh-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.dark .md3-refresh-button {
  border-color: var(--md-outline-variant);
  background: var(--md-surface-container-low);
  color: var(--md-on-surface-variant);
}

.dark .md3-refresh-button:hover:not(:disabled) {
  background: var(--md-state-hover);
  color: var(--md-on-surface);
}

.md3-chart-controls :deep(.date-picker-trigger),
.md3-chart-controls :deep(.select-trigger) {
  min-height: 34px;
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding-top: 0.375rem;
  padding-bottom: 0.375rem;
  box-shadow: none;
}

.dark .md3-chart-controls :deep(.date-picker-trigger),
.dark .md3-chart-controls :deep(.select-trigger) {
  background: var(--md-surface-container-low);
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
  margin-bottom: 12px;
}

.md3-card-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  font-weight: 650;
}

.md3-card-header p {
  margin: 4px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.dark .md3-card-header h2 {
  color: var(--md-on-surface);
}

.dark .md3-card-header p {
  color: var(--md-on-surface-variant);
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
  .md3-charts-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .md3-model-card,
  .md3-quick-actions-card {
    grid-column: 1 / -1;
  }
}

@media (max-width: 720px) {
  .md3-chart-controls {
    width: 100%;
    align-items: stretch;
    flex-direction: column;
  }

  .md3-chart-controls :deep(.relative),
  .md3-chart-controls :deep(.date-picker-trigger),
  .md3-chart-controls :deep(.select-trigger),
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
