<template>
  <section class="md3-stats-shell">
    <div class="md3-stat-grid">
      <article v-if="!isSimple" class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-balance">
          <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" />
          </svg>
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.balance') }}</span>
          <strong class="md3-stat-value md3-stat-value-balance">${{ formatBalance(balance) }}</strong>
          <span class="md3-stat-meta">{{ t('common.available') }}</span>
        </div>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-requests">
          <Icon name="chart" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.todayRequests') }}</span>
          <strong class="md3-stat-value">{{ formatNumber(stats?.today_requests || 0) }}</strong>
          <span class="md3-stat-meta">{{ t('common.total') }}: {{ formatNumber(stats?.total_requests || 0) }}</span>
        </div>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-cost">
          <Icon name="dollar" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.todayCost') }}</span>
          <strong class="md3-stat-value">
            <span class="md3-cost-actual" :title="t('dashboard.actual')">${{ formatCost(stats?.today_actual_cost || 0) }}</span>
            <span class="md3-cost-standard" :title="t('dashboard.standard')"> / ${{ formatCost(stats?.today_cost || 0) }}</span>
          </strong>
          <span class="md3-stat-meta">
            {{ t('common.total') }}:
            <span class="md3-cost-actual" :title="t('dashboard.actual')">${{ formatCost(stats?.total_actual_cost || 0) }}</span>
            <span class="md3-cost-standard" :title="t('dashboard.standard')"> / ${{ formatCost(stats?.total_cost || 0) }}</span>
          </span>
        </div>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-tokens">
          <Icon name="cube" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.todayTokens') }}</span>
          <strong class="md3-stat-value">{{ formatTokens(stats?.today_tokens || 0) }}</strong>
          <span class="md3-stat-meta">
            {{ t('dashboard.input') }}: {{ formatTokens(stats?.today_input_tokens || 0) }} / {{ t('dashboard.output') }}: {{ formatTokens(stats?.today_output_tokens || 0) }}
          </span>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'

defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
}>()
const { t } = useI18n()

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
</script>

<style scoped>
.md3-stats-shell {
  display: grid;
  gap: 16px;
}

.md3-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.md3-stat-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.dark .md3-stat-card {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-stat-card {
  display: grid;
  min-width: 0;
  min-height: 132px;
  align-content: space-between;
  gap: 18px;
  padding: 18px;
}

.md3-stat-icon {
  display: none;
}

.md3-stat-icon svg {
  width: 20px;
  height: 20px;
}

.md3-stat-icon-balance,
.md3-stat-icon-requests,
.md3-stat-icon-cost,
.md3-stat-icon-tokens {
  background: var(--md-surface-container-high);
  color: var(--md-on-surface);
}

.dark .md3-stat-icon-balance,
.dark .md3-stat-icon-requests,
.dark .md3-stat-icon-cost,
.dark .md3-stat-icon-tokens {
  background: var(--md-surface-container-high);
  color: var(--md-on-surface);
}

.md3-stat-copy {
  display: grid;
  min-width: 0;
  gap: 10px;
}

.md3-stat-label {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 500;
  line-height: 1.35;
}

.md3-stat-value {
  min-width: 0;
  color: var(--md-on-surface);
  font-size: 1.5rem;
  line-height: 1.15;
  font-weight: 650;
  letter-spacing: 0;
  word-break: break-word;
}

.md3-stat-value-balance,
.md3-cost-actual {
  color: var(--md-on-surface);
}

.md3-cost-standard {
  color: color-mix(in srgb, var(--md-on-surface-variant) 70%, transparent);
  font-size: 0.875rem;
  font-weight: 600;
}

.md3-stat-meta {
  min-width: 0;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  line-height: 1.45;
}

.dark .md3-stat-label,
.dark .md3-stat-meta {
  color: var(--md-on-surface-variant);
}

.dark .md3-stat-value {
  color: var(--md-on-surface);
}

.dark .md3-stat-value-balance,
.dark .md3-cost-actual {
  color: var(--md-on-surface);
}

.dark .md3-cost-standard {
  color: color-mix(in srgb, var(--md-on-surface-variant) 64%, transparent);
}

@media (max-width: 1200px) {
  .md3-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .md3-stat-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .md3-stat-card {
    min-height: auto;
  }
}
</style>
