<template>
  <section class="md3-usage-panel">
    <header class="md3-panel-header">
      <div>
        <h2>{{ t('dashboard.recentUsage') }}</h2>
        <p>{{ t('dashboard.last7Days') }}</p>
      </div>
      <router-link to="/usage" class="md3-header-link" :title="t('dashboard.viewAllUsage')">
        <span>{{ t('dashboard.viewAllUsage') }}</span>
        <Icon name="arrowRight" size="sm" />
      </router-link>
    </header>

    <div class="md3-panel-body">
      <div v-if="loading" class="md3-loading-state">
        <LoadingSpinner size="lg" />
      </div>
      <div v-else-if="data.length === 0" class="md3-empty-state">
        <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
      </div>
      <div v-else class="md3-usage-list">
        <article v-for="log in data" :key="log.id" class="md3-usage-row">
          <div class="md3-usage-main">
            <span class="md3-usage-icon">
              <Icon name="beaker" size="md" />
            </span>
            <div class="md3-usage-copy">
              <strong :title="log.model">{{ log.model }}</strong>
              <span>{{ formatDateTime(log.created_at) }}</span>
            </div>
          </div>
          <div class="md3-usage-metrics">
            <strong>
              <span class="md3-actual-cost" :title="t('dashboard.actual')">${{ formatCost(log.actual_cost) }}</span>
              <span class="md3-standard-cost" :title="t('dashboard.standard')"> / ${{ formatCost(log.total_cost) }}</span>
            </strong>
            <span>{{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens</span>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>

<style scoped>
.md3-usage-panel {
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.dark .md3-usage-panel {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  border-bottom: 1px solid var(--md-outline-variant);
}

.dark .md3-panel-header {
  border-color: var(--md-outline-variant);
}

.md3-panel-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  font-weight: 760;
}

.md3-panel-header p {
  margin: 4px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.md3-header-link {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  border-radius: 8px;
  padding: 6px 8px;
  color: var(--md-primary);
  font-size: 0.8125rem;
  font-weight: 800;
  transition: background-color 160ms ease;
}

.md3-header-link:hover {
  background: var(--md-state-hover);
}

.dark .md3-panel-header h2 {
  color: var(--md-on-surface);
}

.dark .md3-panel-header p {
  color: var(--md-on-surface-variant);
}

.dark .md3-header-link {
  color: var(--md-primary);
}

.dark .md3-header-link:hover {
  background: var(--md-state-hover);
}

.md3-panel-body {
  padding: 12px;
}

.md3-loading-state,
.md3-empty-state {
  display: flex;
  min-height: 220px;
  align-items: center;
  justify-content: center;
}

.md3-empty-state {
  padding: 24px 8px;
}

.md3-usage-list {
  display: grid;
  gap: 8px;
}

.md3-usage-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 12px;
  transition: background-color 160ms ease, border-color 160ms ease;
}

.md3-usage-row:hover {
  border-color: var(--md-outline-variant);
  background: var(--md-state-hover);
}

.dark .md3-usage-row {
  background: var(--md-surface-container-low);
}

.dark .md3-usage-row:hover {
  border-color: var(--md-outline-variant);
  background: var(--md-state-hover);
}

.md3-usage-main {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.md3-usage-icon {
  display: inline-flex;
  width: 38px;
  height: 38px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: rgb(204 251 241);
  color: rgb(15 118 110);
}

.dark .md3-usage-icon {
  background: rgb(20 184 166 / 0.18);
  color: rgb(94 234 212);
}

.md3-usage-copy {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.md3-usage-copy strong {
  min-width: 0;
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.md3-usage-copy span,
.md3-usage-metrics span {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.md3-usage-metrics {
  display: grid;
  flex: 0 0 auto;
  gap: 3px;
  text-align: right;
}

.md3-usage-metrics strong {
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-variant-numeric: tabular-nums;
  font-weight: 800;
}

.md3-actual-cost {
  color: rgb(5 150 105);
}

.md3-standard-cost {
  color: color-mix(in srgb, var(--md-on-surface-variant) 70%, transparent);
  font-weight: 600;
}

.dark .md3-usage-copy strong,
.dark .md3-usage-metrics strong {
  color: var(--md-on-surface);
}

.dark .md3-usage-copy span,
.dark .md3-usage-metrics span {
  color: var(--md-on-surface-variant);
}

.dark .md3-actual-cost {
  color: rgb(110 231 183);
}

.dark .md3-standard-cost {
  color: color-mix(in srgb, var(--md-on-surface-variant) 64%, transparent);
}

@media (max-width: 640px) {
  .md3-panel-header,
  .md3-usage-row {
    align-items: stretch;
    flex-direction: column;
  }

  .md3-header-link {
    align-self: flex-start;
  }

  .md3-usage-metrics {
    width: 100%;
    text-align: left;
  }
}
</style>
