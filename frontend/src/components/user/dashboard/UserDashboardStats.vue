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
        <span class="md3-stat-icon md3-stat-icon-key">
          <Icon name="key" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.apiKeys') }}</span>
          <strong class="md3-stat-value">{{ stats?.total_api_keys || 0 }}</strong>
          <span class="md3-stat-meta md3-stat-meta-success">{{ stats?.active_api_keys || 0 }} {{ t('common.active') }}</span>
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
    </div>

    <div class="md3-stat-grid md3-stat-grid-secondary">
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

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-database">
          <Icon name="database" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.totalTokens') }}</span>
          <strong class="md3-stat-value">{{ formatTokens(stats?.total_tokens || 0) }}</strong>
          <span class="md3-stat-meta">
            {{ t('dashboard.input') }}: {{ formatTokens(stats?.total_input_tokens || 0) }} / {{ t('dashboard.output') }}: {{ formatTokens(stats?.total_output_tokens || 0) }}
          </span>
        </div>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-performance">
          <Icon name="bolt" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.performance') }}</span>
          <strong class="md3-stat-value md3-inline-metric">
            {{ formatTokens(stats?.rpm || 0) }}
            <span>RPM</span>
          </strong>
          <span class="md3-stat-meta md3-stat-meta-accent">{{ formatTokens(stats?.tpm || 0) }} TPM</span>
        </div>
      </article>

      <article class="md3-stat-card">
        <span class="md3-stat-icon md3-stat-icon-latency">
          <Icon name="clock" size="md" :stroke-width="2" />
        </span>
        <div class="md3-stat-copy">
          <span class="md3-stat-label">{{ t('dashboard.avgResponse') }}</span>
          <strong class="md3-stat-value">{{ formatDuration(stats?.average_duration_ms || 0) }}</strong>
          <span class="md3-stat-meta">{{ t('dashboard.averageTime') }}</span>
        </div>
      </article>
    </div>

    <section v-if="!isSimple && platformCards.length > 0" class="md3-platform-panel">
      <header class="md3-section-header">
        <div>
          <h2>{{ t('dashboard.platformBreakdown') }}</h2>
          <p>{{ t('dashboard.platformCount', { count: sortedPlatforms.length }) }}</p>
        </div>
      </header>

      <div class="md3-platform-grid">
        <article
          v-for="item in platformCards"
          :key="item.platform"
          class="md3-platform-card"
          :class="{ 'md3-platform-card-other': item.isOther }"
        >
          <div class="md3-platform-card-header">
            <span class="md3-platform-name">
              {{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}
            </span>
            <strong class="md3-platform-total" :title="t('dashboard.actual')">
              ${{ formatCost(item.total_actual_cost) }}
            </strong>
          </div>

          <div class="md3-kv-list">
            <div>
              <span>{{ t('dashboard.todayCost') }}</span>
              <strong>${{ formatCost(item.today_actual_cost) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.requests') }}</span>
              <strong>{{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.tokens') }}</span>
              <strong>{{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}</strong>
            </div>
          </div>

          <!-- Quota 区：仅当 quota 配置存在、非 __other__ 且至少有一个窗口配了 limit 时显示 -->
          <div v-if="hasAnyLimit(item.quota) && !item.isOther" class="md3-quota-block">
            <p>{{ t('dashboard.platformQuota.title') }}</p>
            <template v-for="w in (['daily', 'weekly', 'monthly'] as const)" :key="w">
              <div v-if="quotaVal(item.quota, `${w}_limit_usd`) != null" class="md3-quota-window">
                <!-- limit=0：完全禁用 -->
                <template v-if="(quotaVal(item.quota, `${w}_limit_usd`) as number) === 0">
                  <div class="md3-quota-row">
                    <span>{{ t(`dashboard.platformQuota.${w}`) }}</span>
                    <strong class="md3-quota-disabled">{{ t('dashboard.platformQuota.disabled') }}</strong>
                  </div>
                  <div class="md3-quota-track">
                    <div class="md3-quota-fill md3-quota-fill-disabled" />
                  </div>
                </template>
                <!-- limit>0：正常用量进度条 -->
                <template v-else>
                  <div class="md3-quota-row">
                    <span>{{ t(`dashboard.platformQuota.${w}`) }}</span>
                    <strong>
                      ${{ formatUsd((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0) }} / ${{ formatUsd(quotaVal(item.quota, `${w}_limit_usd`) as number) }}
                    </strong>
                  </div>
                  <div class="md3-quota-track">
                    <div
                      class="md3-quota-fill"
                      :class="quotaBarClass(calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number))"
                      :style="{ width: calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) + '%' }"
                    />
                  </div>
                  <p v-if="quotaVal(item.quota, `${w}_window_resets_at`)" class="md3-quota-reset">
                    {{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(quotaVal(item.quota, `${w}_window_resets_at`) as string) }) }}
                  </p>
                </template>
              </div>
            </template>
          </div>
        </article>
      </div>
    </section>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

interface FusedPlatformCard {
  platform: string
  total_actual_cost: number
  today_actual_cost: number
  total_requests: number
  total_tokens: number
  isOther?: boolean
  quota?: PlatformQuotaItem
}

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  platformQuotas?: PlatformQuotaItem[] | null
}>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  kiro: 'Kiro'
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

const sortedPlatforms = computed(() => {
  const list = props.stats?.by_platform ?? []
  return [...list].sort((a, b) => b.total_actual_cost - a.total_actual_cost)
})

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, (typeof sortedPlatforms.value)[number]>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // union 平台集合。后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>([...byPlat.keys(), ...byQuota.keys()])

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'kiro']
  const cards: FusedPlatformCard[] = []

  for (const p of platforms) {
    const stat = byPlat.get(p)
    cards.push({
      platform: p,
      total_actual_cost: stat?.total_actual_cost ?? 0,
      today_actual_cost: stat?.today_actual_cost ?? 0,
      total_requests: stat?.total_requests ?? 0,
      total_tokens: stat?.total_tokens ?? 0,
      quota: byQuota.get(p),
    })
  }

  // 排序：按 PLATFORM_ORDER，未知平台按名称排序
  cards.sort((a, b) => {
    const ai = PLATFORM_ORDER.indexOf(a.platform)
    const bi = PLATFORM_ORDER.indexOf(b.platform)
    if (ai === -1 && bi === -1) return a.platform.localeCompare(b.platform)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  // __other__ 补差逻辑：只对 by_platform 有 usage 数据的总和计算
  const total = props.stats?.total_actual_cost ?? 0
  const today = props.stats?.today_actual_cost ?? 0
  const sumTotal = cards.reduce((s, c) => s + c.total_actual_cost, 0)
  const sumToday = cards.reduce((s, c) => s + c.today_actual_cost, 0)
  const diffTotal = Math.max(0, total - sumTotal)
  const diffToday = Math.max(0, today - sumToday)

  if (diffTotal > OTHER_THRESHOLD || diffToday > OTHER_THRESHOLD) {
    cards.push({
      platform: '__other__',
      total_actual_cost: diffTotal,
      today_actual_cost: diffToday,
      total_requests: 0,
      total_tokens: 0,
      isOther: true,
    })
  }

  return cards
})

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'
type QuotaField = `${QuotaWindow}_limit_usd` | `${QuotaWindow}_usage_usd` | `${QuotaWindow}_window_resets_at`

function quotaVal(q: PlatformQuotaItem | undefined, key: QuotaField): PlatformQuotaItem[QuotaField] {
  return q?.[key]
}

function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
}

function calcPercent(usage: number, limit: number): number {
  if (!limit || limit <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((usage / limit) * 100)))
}

function quotaBarClass(p: number): string {
  if (p >= 95) return 'bg-red-500'
  if (p >= 75) return 'bg-amber-500'
  return 'bg-green-500'
}

// 与 formatBalance 一致使用 Intl.NumberFormat 做半偶舍入，避免 toFixed 在不同 JS 引擎
// 下偶发截断而非四舍五入（与后端展示精度不一致）。
const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
function formatUsd(n: number): string {
  if (!Number.isFinite(n)) return '0.00'
  return usdFormatter.format(n)
}

function formatResetTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

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
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`
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

.md3-stat-card,
.md3-platform-panel {
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.dark .md3-stat-card,
.dark .md3-platform-panel {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.md3-stat-card {
  display: flex;
  min-width: 0;
  min-height: 116px;
  align-items: flex-start;
  gap: 14px;
  padding: 16px;
}

.md3-stat-icon {
  display: inline-flex;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
}

.md3-stat-icon svg {
  width: 20px;
  height: 20px;
}

.md3-stat-icon-balance {
  background: rgb(209 250 229);
  color: rgb(4 120 87);
}

.md3-stat-icon-key {
  background: rgb(219 234 254);
  color: rgb(29 78 216);
}

.md3-stat-icon-requests {
  background: rgb(204 251 241);
  color: rgb(15 118 110);
}

.md3-stat-icon-cost {
  background: rgb(237 233 254);
  color: rgb(109 40 217);
}

.md3-stat-icon-tokens {
  background: rgb(254 243 199);
  color: rgb(180 83 9);
}

.md3-stat-icon-database {
  background: rgb(224 231 255);
  color: rgb(67 56 202);
}

.md3-stat-icon-performance {
  background: rgb(224 242 254);
  color: rgb(3 105 161);
}

.md3-stat-icon-latency {
  background: rgb(255 228 230);
  color: rgb(190 18 60);
}

.dark .md3-stat-icon-balance {
  background: rgb(6 95 70 / 0.28);
  color: rgb(110 231 183);
}

.dark .md3-stat-icon-key {
  background: rgb(37 99 235 / 0.22);
  color: rgb(147 197 253);
}

.dark .md3-stat-icon-requests {
  background: rgb(20 184 166 / 0.18);
  color: rgb(94 234 212);
}

.dark .md3-stat-icon-cost {
  background: rgb(124 58 237 / 0.22);
  color: rgb(196 181 253);
}

.dark .md3-stat-icon-tokens {
  background: rgb(217 119 6 / 0.22);
  color: rgb(252 211 77);
}

.dark .md3-stat-icon-database {
  background: rgb(79 70 229 / 0.24);
  color: rgb(165 180 252);
}

.dark .md3-stat-icon-performance {
  background: rgb(14 116 144 / 0.24);
  color: rgb(103 232 249);
}

.dark .md3-stat-icon-latency {
  background: rgb(190 18 60 / 0.22);
  color: rgb(253 164 175);
}

.md3-stat-copy {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.md3-stat-label {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 700;
}

.md3-stat-value {
  min-width: 0;
  color: var(--md-on-surface);
  font-size: 1.25rem;
  line-height: 1.25;
  font-weight: 760;
  word-break: break-word;
}

.md3-stat-value-balance,
.md3-cost-actual {
  color: rgb(13 148 136);
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

.md3-stat-meta-success {
  color: rgb(5 150 105);
  font-weight: 700;
}

.md3-stat-meta-accent {
  color: rgb(3 105 161);
  font-weight: 700;
}

.md3-inline-metric {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.md3-inline-metric span {
  color: var(--md-on-surface-variant);
  font-size: 0.6875rem;
  font-weight: 700;
}

.dark .md3-stat-label,
.dark .md3-stat-meta,
.dark .md3-inline-metric span {
  color: var(--md-on-surface-variant);
}

.dark .md3-stat-value {
  color: var(--md-on-surface);
}

.dark .md3-stat-value-balance,
.dark .md3-cost-actual {
  color: rgb(94 234 212);
}

.dark .md3-cost-standard {
  color: color-mix(in srgb, var(--md-on-surface-variant) 64%, transparent);
}

.dark .md3-stat-meta-success {
  color: rgb(110 231 183);
}

.dark .md3-stat-meta-accent {
  color: rgb(103 232 249);
}

.md3-platform-panel {
  padding: 18px;
}

.md3-section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.md3-section-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 0.9375rem;
  font-weight: 760;
}

.md3-section-header p {
  margin: 4px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.dark .md3-section-header h2 {
  color: var(--md-on-surface);
}

.dark .md3-section-header p {
  color: var(--md-on-surface-variant);
}

.md3-platform-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.md3-platform-card {
  min-width: 0;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 14px;
}

.md3-platform-card-other {
  border-style: dashed;
  background: var(--md-surface-container);
}

.dark .md3-platform-card {
  border-color: var(--md-outline-variant);
  background: var(--md-surface-container-low);
}

.dark .md3-platform-card-other {
  background: var(--md-surface-container);
}

.md3-platform-card-header {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.md3-platform-name {
  min-width: 0;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 760;
  overflow-wrap: anywhere;
}

.md3-platform-total {
  color: rgb(109 40 217);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  white-space: nowrap;
}

.dark .md3-platform-name {
  color: var(--md-on-surface);
}

.dark .md3-platform-total {
  color: rgb(196 181 253);
}

.md3-kv-list {
  display: grid;
  gap: 6px;
  margin-top: 12px;
}

.md3-kv-list div,
.md3-quota-row {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.md3-kv-list strong,
.md3-quota-row strong {
  color: var(--md-on-surface);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 700;
  text-align: right;
  overflow-wrap: anywhere;
}

.dark .md3-kv-list div,
.dark .md3-quota-row {
  color: var(--md-on-surface-variant);
}

.dark .md3-kv-list strong,
.dark .md3-quota-row strong {
  color: var(--md-on-surface);
}

.md3-quota-block {
  display: grid;
  gap: 8px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--md-outline-variant);
}

.dark .md3-quota-block {
  border-color: var(--md-outline-variant);
}

.md3-quota-block > p {
  margin: 0;
  color: var(--md-on-surface-variant);
  font-size: 0.6875rem;
  font-weight: 800;
  text-transform: uppercase;
}

.md3-quota-window {
  display: grid;
  gap: 5px;
}

.md3-quota-disabled {
  color: rgb(220 38 38) !important;
}

.md3-quota-track {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--md-surface-container-high);
}

.dark .md3-quota-track {
  background: var(--md-surface-container-high);
}

.md3-quota-fill {
  height: 100%;
  border-radius: inherit;
  transition: width 180ms ease;
}

.md3-quota-fill.bg-red-500,
.md3-quota-fill-disabled {
  background: rgb(239 68 68);
}

.md3-quota-fill.bg-amber-500 {
  background: rgb(245 158 11);
}

.md3-quota-fill.bg-green-500 {
  background: rgb(16 185 129);
}

.md3-quota-reset {
  margin: 0;
  color: color-mix(in srgb, var(--md-on-surface-variant) 70%, transparent);
  font-size: 0.6875rem;
}

@media (max-width: 1200px) {
  .md3-stat-grid,
  .md3-platform-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .md3-stat-grid,
  .md3-platform-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .md3-stat-card {
    min-height: auto;
  }
}
</style>
