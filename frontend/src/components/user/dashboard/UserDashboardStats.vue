<template>
  <section class="md3-stats-shell">
    <div class="md3-stat-grid">
      <article v-if="!isSimple" class="md3-stat-card">
        <span class="md3-stat-label">{{ t('dashboard.balance') }}</span>
        <strong class="md3-stat-value">${{ formatBalance(balance) }}</strong>
        <div class="md3-balance-breakdown">
          <div class="md3-balance-row">
            <span>{{ t('common.rechargeBalance') }}</span>
            <strong>${{ formatBalance(rechargeBalance) }}</strong>
          </div>
          <div class="md3-balance-row md3-balance-gift">
            <span>{{ t('common.giftBalance') }}</span>
            <strong>${{ formatBalance(giftBalance) }}</strong>
          </div>
          <div class="md3-balance-sources">
            <span>{{ t('common.registrationGift') }} ${{ formatBalance(registrationGiftBalance) }}</span>
            <span>{{ t('common.dailyCheckinGift') }} ${{ formatBalance(dailyCheckinBalance) }}</span>
          </div>
          <span v-if="giftBalanceExpiresAt" class="md3-stat-meta">
            {{ t('common.giftExpiresAt', { date: formatGiftExpiry(giftBalanceExpiresAt) }) }}
          </span>
        </div>
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
  <!-- Row 3: Per-platform breakdown -->
  <div v-if="!isSimple && platformCards.length > 0" class="card p-4">
    <div class="mb-3 flex items-center justify-between">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.platformBreakdown') }}</h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('dashboard.platformCount', { count: platformCount }) }}
      </span>
    </div>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div
        v-for="item in platformCards"
        :key="item.platform"
        data-testid="platform-card"
        :data-platform="item.platform"
        :class="[
          'rounded-lg border p-3',
          item.isOther
            ? 'border-dashed border-gray-300 bg-gray-50 dark:border-dark-500 dark:bg-dark-700/30'
            : 'border-gray-200 dark:border-dark-600'
        ]"
      >
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}
          </span>
          <span class="font-mono text-sm text-purple-600 dark:text-purple-400" :title="t('dashboard.actual')">
            ${{ formatCost(item.total_actual_cost) }}
          </span>
        </div>
        <div class="mt-2 space-y-1 text-xs">
          <div class="flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.todayCost') }}</span>
            <span class="font-mono text-gray-900 dark:text-white">${{ formatCost(item.today_actual_cost) }}</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.requests') }}</span>
            <span class="font-mono text-gray-700 dark:text-gray-300">
              {{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}
            </span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('dashboard.tokens') }}</span>
            <span class="font-mono text-gray-700 dark:text-gray-300">
              {{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}
            </span>
          </div>
        </div>

        <!-- Quota 区：仅当 quota 配置存在、非 __other__ 且至少有一个窗口配了 limit 时显示 -->
        <div v-if="hasAnyLimit(item.quota) && !item.isOther" class="mt-3 space-y-1.5 border-t border-gray-200 pt-2 dark:border-dark-700">
          <p class="text-[10px] uppercase tracking-wide text-gray-400">
            {{ t('dashboard.platformQuota.title') }}
          </p>
          <template v-for="w in (['daily', 'weekly', 'monthly'] as const)" :key="w">
            <div v-if="quotaVal(item.quota, `${w}_limit_usd`) != null" class="space-y-0.5">
              <!-- limit=0：完全禁用 -->
              <template v-if="(quotaVal(item.quota, `${w}_limit_usd`) as number) === 0">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-gray-600 dark:text-gray-300">{{ t(`dashboard.platformQuota.${w}`) }}</span>
                  <span class="font-mono text-red-500">{{ t('dashboard.platformQuota.disabled') }}</span>
                </div>
                <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                  <div class="h-full w-full rounded-full bg-red-500" />
                </div>
              </template>
              <!-- limit>0：正常用量进度条 -->
              <template v-else>
                <div class="flex items-center justify-between text-xs">
                  <span class="text-gray-600 dark:text-gray-300">{{ t(`dashboard.platformQuota.${w}`) }}</span>
                  <span class="font-mono text-gray-700 dark:text-gray-200">
                    ${{ formatUsd((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0) }} / ${{ formatUsd(quotaVal(item.quota, `${w}_limit_usd`) as number) }}
                  </span>
                </div>
                <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                  <div
                    class="h-full rounded-full transition-all"
                    :class="quotaBarClass(calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number))"
                    :style="{ width: calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) + '%' }"
                  />
                </div>
                <p v-if="quotaVal(item.quota, `${w}_window_resets_at`)" class="text-[10px] text-gray-400">
                  {{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(quotaVal(item.quota, `${w}_window_resets_at`) as string) }) }}
                </p>
              </template>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlatformDashboardStats, UserDashboardStats as UserStatsType } from '@/api/usage'
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
  rechargeBalance?: number
  giftBalance?: number
  registrationGiftBalance?: number
  dailyCheckinBalance?: number
  giftBalanceExpiresAt?: string | null
  platformQuotas?: PlatformQuotaItem[] | null
  isSimple: boolean
}>()
const { t } = useI18n()

const stats = computed(() => props.stats)
const balance = computed(() => props.balance)
const isSimple = computed(() => props.isSimple)
const rechargeBalance = computed(() => Number(props.rechargeBalance ?? Math.max(props.balance - Number(props.giftBalance || 0), 0)))
const giftBalance = computed(() => Number(props.giftBalance || 0))
const registrationGiftBalance = computed(() => Number(props.registrationGiftBalance || 0))
const dailyCheckinBalance = computed(() => Number(props.dailyCheckinBalance || 0))
const giftBalanceExpiresAt = computed(() => props.giftBalanceExpiresAt || null)
const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok',
  kimi: 'Kimi',
  zhipu: 'Zhipu GLM',
  deepseek: 'DeepSeek',
  minimax: 'MiniMax',
  typesafe: 'TypeSafe / Jev',
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, PlatformDashboardStats>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map。三档全空的记录不产生卡片，挂到卡片上也不渲染配额区。
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // 卡片集合 = 有用量的平台 ∪ 至少配置了一档限额的平台。
  // 三档全空的限额记录等价于不限额，不单独产生卡片。
  // 后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>(byPlat.keys())
  for (const [platform, q] of byQuota) {
    if (hasAnyLimit(q)) platforms.add(platform)
  }

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'typesafe']
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

// 标题右侧的平台计数 = 实际渲染的平台卡片数，不含"其他"差额卡。
const platformCount = computed(() => platformCards.value.filter((c) => !c.isOther).length)

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

const formatGiftExpiry = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
  }).format(date)
}

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

.md3-balance-breakdown {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.md3-balance-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  line-height: 1.35;
}

.md3-balance-row strong {
  color: var(--md-on-surface);
  font-weight: 600;
}

.md3-balance-row.md3-balance-gift strong {
  color: var(--md-success);
}

.md3-balance-sources {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  color: var(--md-on-surface-variant);
  font-size: 0.6875rem;
  line-height: 1.35;
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
