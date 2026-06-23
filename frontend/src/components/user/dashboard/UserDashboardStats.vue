<template>
  <section class="md3-stats-shell">
    <div class="md3-stat-grid">
      <article v-if="!isSimple" class="md3-stat-card">
        <span class="md3-stat-label">{{ t('dashboard.balance') }}</span>
        <strong class="md3-stat-value">${{ formatBalance(balance) }}</strong>
        <span class="md3-stat-meta">{{ t('common.available') }}</span>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-label">{{ t('dashboard.todayRequests') }}</span>
        <strong class="md3-stat-value">{{ formatNumber(stats?.today_requests || 0) }}</strong>
        <span class="md3-stat-meta">{{ t('common.total') }}: {{ formatNumber(stats?.total_requests || 0) }}</span>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-label">{{ t('dashboard.todayCost') }}</span>
        <strong class="md3-stat-value">${{ formatCost(stats?.today_actual_cost || 0) }}</strong>
        <span class="md3-stat-meta">
          {{ t('dashboard.standard') }} ${{ formatCost(stats?.today_cost || 0) }} ·
          {{ t('common.total') }} ${{ formatCost(stats?.total_actual_cost || 0) }}
        </span>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-label">{{ t('dashboard.todayTokens') }}</span>
        <strong class="md3-stat-value">{{ formatTokens(stats?.today_tokens || 0) }}</strong>
        <span class="md3-stat-meta">
          <span class="md3-token-input">{{ t('dashboard.input') }} {{ formatTokens(stats?.today_input_tokens || 0) }}</span>
          <span> · </span>
          <span class="md3-token-output">{{ t('dashboard.output') }} {{ formatTokens(stats?.today_output_tokens || 0) }}</span>
        </span>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
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
  min-width: 0;
  display: grid;
  min-height: 116px;
  grid-template-rows: auto 1fr auto;
  gap: 12px;
  padding: 16px;
}

.md3-stat-label {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 450;
  line-height: 1.35;
}

.md3-stat-value {
  min-width: 0;
  color: var(--md-on-surface);
  align-self: center;
  font-size: 1.625rem;
  line-height: 1.15;
  font-weight: 650;
  letter-spacing: 0;
  word-break: break-word;
}

.md3-stat-meta {
  min-width: 0;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  line-height: 1.45;
}

.md3-token-input {
  color: var(--md-token-input);
}

.md3-token-output {
  color: var(--md-token-output);
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
