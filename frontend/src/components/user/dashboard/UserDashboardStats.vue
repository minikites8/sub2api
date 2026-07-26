<template>
  <section class="telemetry-stats">
    <div class="telemetry-stat-grid">
      <article class="telemetry-stat-card telemetry-stat-card--green">
        <span class="telemetry-stat-label">{{ t('dashboard.totalRequestsAll') }}</span>
        <strong class="telemetry-stat-value">{{ formatCompact(stats.total_requests || 0) }}</strong>
        <span class="telemetry-stat-meta telemetry-stat-meta--green">
          <Icon name="trendingUp" size="xs" />
          {{ t('dashboard.todayRequests') }} {{ formatNumber(stats.today_requests || 0) }}
        </span>
      </article>

      <article class="telemetry-stat-card telemetry-stat-card--blue">
        <span class="telemetry-stat-label">{{ t('dashboard.avgLatency') }}</span>
        <strong class="telemetry-stat-value">
          {{ formatNumber(Math.round(stats.average_duration_ms || 0)) }}<small>ms</small>
        </strong>
        <span class="telemetry-stat-meta telemetry-stat-meta--blue">
          <Icon name="bolt" size="xs" />
          RPM {{ formatNumber(stats.rpm || 0) }}
        </span>
      </article>

      <article class="telemetry-stat-card telemetry-stat-card--neutral">
        <span class="telemetry-stat-label">{{ t('dashboard.totalTokensAll') }}</span>
        <strong class="telemetry-stat-value">{{ formatTokens(stats.total_tokens || 0) }}</strong>
        <span class="telemetry-stat-meta">
          {{ t('dashboard.actual') }} {{ formatCreditValue(stats.total_actual_cost || 0) }}
          <span class="telemetry-stat-meta-sep">·</span>
          <span class="telemetry-stat-meta--green">
            {{ t('dashboard.cacheHitRate') }} {{ formatPercent(cacheHitRate) }}
          </span>
        </span>
      </article>

      <article class="telemetry-stat-card telemetry-stat-card--live">
        <span class="telemetry-live-dot" aria-hidden="true"></span>
        <span class="telemetry-stat-label">
          {{ isSimple ? t('dashboard.activeKeys') : t('dashboard.availableBalance') }}
        </span>
        <strong class="telemetry-stat-value">
          {{ isSimple ? formatNumber(stats.active_api_keys || 0) : formatCreditAmount(balance) }}<small v-if="!isSimple">Credits</small>
        </strong>
        <span class="telemetry-stat-meta">
          {{ stats.active_api_keys || 0 }} / {{ stats.total_api_keys || 0 }} {{ t('common.active') }}
        </span>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatCredits, usdToCredits } from '@/utils/credit'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
}>()
const { t } = useI18n()

// 与后端 cacheHitRate 保持同一公式（usage_log_repo_trend.go）：
// 命中率 = 缓存读取 / (输入 + 缓存创建 + 缓存读取)。分母用 prompt 侧
// token 而不是 total_tokens，因为输出 token 不经过缓存。
const cacheHitRate = computed(() => {
  const s = props.stats
  const cacheRead = s.total_cache_read_tokens || 0
  const promptTokens = (s.total_input_tokens || 0) + (s.total_cache_creation_tokens || 0) + cacheRead
  if (promptTokens <= 0 || cacheRead <= 0) return 0
  return (cacheRead / promptTokens) * 100
})

const formatPercent = (value: number) => `${value.toFixed(1)}%`

// Amounts come from the API in USD; the UI presents everything in Credits.
const formatCreditAmount = (usd: number) => formatCredits(usdToCredits(usd))
const formatCreditValue = (usd: number) => `${formatCreditAmount(usd)} Credits`

const formatNumber = (n: number) => n.toLocaleString()
const formatCompact = (value: number) =>
  new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 2 }).format(value)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
</script>

<style scoped>
.telemetry-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 20px;
}

.telemetry-stat-card {
  position: relative;
  min-width: 0;
  display: grid;
  min-height: 150px;
  grid-template-rows: auto 1fr auto;
  gap: 14px;
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 6px;
  background: var(--md-surface-container-low);
  padding: 22px;
  transition: border-color 180ms ease, transform 180ms ease;
}

.telemetry-stat-card::after {
  position: absolute;
  top: -36px;
  right: -36px;
  width: 92px;
  height: 92px;
  border-radius: 50%;
  background: rgb(0 227 139 / 5%);
  content: '';
}

.telemetry-stat-card--blue::after {
  background: rgb(80 142 255 / 7%);
}

.telemetry-stat-card:hover {
  border-color: #00e38b;
  transform: translateY(-1px);
}

.telemetry-stat-card--live {
  box-shadow: 0 0 18px rgb(0 227 139 / 7%);
}

.telemetry-stat-label {
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.66rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.telemetry-stat-value {
  min-width: 0;
  color: var(--md-on-surface);
  align-self: center;
  font-size: 2rem;
  line-height: 1;
  font-weight: 700;
  letter-spacing: 0;
  word-break: break-word;
}

.telemetry-stat-value small {
  margin-left: 3px;
  color: var(--md-on-surface-variant);
  font-size: 0.9rem;
  font-weight: 600;
}

.telemetry-stat-meta {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  color: var(--md-on-surface-variant);
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.66rem;
  line-height: 1.45;
}

.telemetry-stat-meta--green {
  color: #00e38b;
}

.telemetry-stat-meta-sep {
  color: var(--md-outline);
}

.telemetry-stat-meta--blue {
  color: #8eb5ff;
}

.telemetry-live-dot {
  position: absolute;
  top: 18px;
  right: 18px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #00e38b;
  box-shadow: 0 0 10px rgb(0 227 139 / 55%);
  animation: telemetry-pulse 2s ease-in-out infinite;
}

@keyframes telemetry-pulse {
  50% { opacity: 0.45; }
}

@media (max-width: 1200px) {
  .telemetry-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .telemetry-stat-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .telemetry-stat-card {
    min-height: auto;
  }
}
</style>
