<template>
  <AppLayout>
    <div class="usage-analytics">
      <header class="usage-analytics__hero">
        <div>
          <div class="usage-analytics__live"><span />{{ t('usage.analytics.live') }}</div>
          <h1>{{ t('usage.analytics.title') }}</h1>
          <p>{{ t('usage.analytics.subtitle') }}</p>
        </div>
        <div class="usage-range-control">
          <button type="button" :class="{ active: activeRangePreset === '7' }" @click="applyDatePreset(7)">
            {{ t('usage.analytics.last7Days') }}
          </button>
          <button type="button" :class="{ active: activeRangePreset === '30' }" @click="applyDatePreset(30)">
            {{ t('usage.analytics.last30Days') }}
          </button>
          <div class="usage-range-control__custom" :class="{ active: activeRangePreset === 'custom' }">
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="onDateRangeChange"
            />
          </div>
        </div>
      </header>

      <div class="usage-credit-rule">
        <Icon name="creditCard" size="sm" />
        <span>{{ t('usage.analytics.billingRule') }}</span>
      </div>

      <section class="usage-kpi-grid" aria-label="Usage summary">
        <article class="usage-kpi-card usage-kpi-card--primary">
          <div class="usage-kpi-card__label"><span>{{ t('usage.analytics.totalCreditConsumption') }}</span><Icon name="bolt" size="sm" /></div>
          <strong>{{ formatCreditValue(totalActualCredits) }}</strong>
          <p>{{ formatNumber(usageStats?.total_requests || 0) }} {{ t('usage.analytics.requestsProcessed') }}</p>
        </article>
        <article class="usage-kpi-card">
          <div class="usage-kpi-card__label">
            <span>{{ t(projectedPeriodDays === 7 ? 'usage.analytics.estimated7DayCredits' : 'usage.analytics.estimated30DayCredits') }}</span>
            <Icon name="calculator" size="sm" />
          </div>
          <strong>{{ formatCreditValue(projectedPeriodCredits) }}</strong>
          <p>{{ t('usage.analytics.periodProjectionHint', { days: projectedPeriodDays }) }}</p>
        </article>
        <article class="usage-kpi-card" :class="{ 'usage-kpi-card--warning': averageLatency >= LATENCY_SLOW_MS }">
          <div class="usage-kpi-card__label"><span>{{ t('usage.analytics.averageLatency') }}</span><Icon name="clock" size="sm" /></div>
          <strong>{{ formatLatency(averageLatency) }}</strong>
          <p>{{ formatCompactNumber(usageStats?.total_tokens || 0) }} {{ t('usage.analytics.tokensProcessed') }}</p>
        </article>
      </section>

      <div class="usage-chart-grid">
        <UsageTokenBarChart :trend-data="trendData" :loading="chartsLoading" />
        <UsageModelCreditChart :model-stats="requestedModelStats" :loading="modelStatsLoading" />
      </div>

      <UsageLatencyHeatmap
        :logs="analyticsLogs"
        :end-date="endDate"
        :period-days="projectedPeriodDays"
      />

      <section class="usage-panel usage-log-panel">
        <div class="usage-log-panel__topbar">
          <div>
            <p class="usage-panel__eyebrow">REQUEST AUDIT</p>
            <h2>{{ t('usage.analytics.recentRequests') }}</h2>
          </div>
          <div class="usage-log-actions">
            <label class="usage-search">
              <Icon name="search" size="sm" />
              <input v-model="logSearch" type="search" :placeholder="t('usage.analytics.searchRequests')" />
            </label>
            <button type="button" class="usage-icon-button" :class="{ active: showAdvancedFilters }" :title="t('usage.analytics.filters')" @click="showAdvancedFilters = !showAdvancedFilters">
              <Icon name="filter" size="sm" />
            </button>
            <button type="button" class="usage-icon-button" :title="t('common.refresh')" :disabled="activeTab === 'errors' ? errorLoading : loading" @click="refreshData">
              <Icon name="refresh" size="sm" />
            </button>
            <button v-if="activeTab === 'usage'" type="button" class="usage-icon-button" :title="t('usage.exportCsv')" :disabled="exporting" @click="exportToCSV">
              <Icon name="download" size="sm" />
            </button>
          </div>
        </div>

        <div v-if="errorViewEnabled" class="usage-tabs">
          <button type="button" :class="{ active: activeTab === 'usage' }" @click="activeTab = 'usage'">{{ t('usage.tabs.usage') }}</button>
          <button type="button" :class="{ active: activeTab === 'errors' }" @click="switchToErrors">{{ t('usage.tabs.errors') }}</button>
        </div>

        <div v-if="showAdvancedFilters" class="usage-filter-panel">
          <template v-if="activeTab === 'usage'">
            <div><label>{{ t('usage.apiKeyFilter') }}</label><Select v-model="filters.api_key_id" :options="apiKeyOptions" @change="applyFilters" /></div>
            <div><label>{{ t('usage.model') }}</label><Select v-model="filters.model" :options="modelOptions" searchable @change="applyFilters" /></div>
            <div><label>{{ t('admin.usage.group') }}</label><Select v-model="filters.group_id" :options="groupOptions" searchable @change="applyFilters" /></div>
            <div><label>{{ t('usage.type') }}</label><Select v-model="filters.request_type" :options="requestTypeOptions" @change="applyFilters" /></div>
            <div><label>{{ t('admin.usage.billingType') }}</label><Select v-model="filters.billing_type" :options="billingTypeOptions" @change="applyFilters" /></div>
            <div><label>{{ t('admin.usage.billingMode') }}</label><Select v-model="filters.billing_mode" :options="billingModeOptions" @change="applyFilters" /></div>
          </template>
          <template v-else>
            <div><label>{{ t('usage.errors.keyName') }}</label><Select v-model="errorFilter.api_key_id" :options="errorKeyOptions" @change="applyErrorFilters" /></div>
            <div><label>{{ t('usage.errors.model') }}</label><Select v-model="errorFilter.model" :options="errorModelOptions" searchable creatable clearable :placeholder="t('usage.errors.modelPlaceholder')" @change="applyErrorFilters" /></div>
            <div><label>{{ t('usage.errors.category') }}</label><Select v-model="errorFilter.category" :options="errorCategoryOptions" @change="applyErrorFilters" /></div>
            <div><label>{{ t('usage.errors.status') }}</label><Select v-model="errorFilter.status_code" :options="errorStatusOptions" @change="applyErrorFilters" /></div>
          </template>
          <button type="button" class="usage-reset-button" @click="resetFilters">{{ t('common.reset') }}</button>
        </div>

        <template v-if="activeTab === 'usage'">
          <div class="usage-table-scroll">
            <table class="usage-log-table">
              <thead>
                <tr>
                  <th>{{ t('usage.analytics.timestamp') }}</th>
                  <th>{{ t('usage.model') }}</th>
                  <th>{{ t('admin.usage.group') }}</th>
                  <th>{{ t('usage.tokens') }}</th>
                  <th>{{ t('usage.latency') }}</th>
                  <th>{{ t('usage.analytics.status') }}</th>
                  <th>{{ t('usage.analytics.credits') }}</th>
                  <th class="usage-log-table__action-heading">{{ t('usage.requestDetails.action') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="loading">
                  <td colspan="8" class="usage-log-table__empty"><LoadingSpinner /></td>
                </tr>
                <tr v-for="log in filteredUsageLogs" v-else :key="log.id">
                  <td class="usage-log-table__time">
                    <strong>{{ formatLogDate(log.created_at) }}</strong>
                    <span>{{ formatLogTime(log.created_at) }}</span>
                  </td>
                  <td><span class="usage-model-pill">{{ log.model }}</span><small>{{ log.api_key?.name || t('usage.analytics.unknownKey') }}</small></td>
                  <td><span class="usage-group-label">{{ formatLogGroup(log) }}</span></td>
                  <td>
                    <div class="usage-token-cell">
                      <span>{{ formatNumber(totalLogTokens(log)) }}</span>
                      <button
                        type="button"
                        class="usage-token-detail-trigger"
                        data-testid="token-details-trigger"
                        :aria-label="t('usage.tokenDetails')"
                        @mouseenter="showTokenTooltip($event, log)"
                        @mouseleave="hideTokenTooltip"
                        @focus="showTokenTooltip($event, log)"
                        @blur="hideTokenTooltip"
                        @click.stop="toggleTokenTooltip($event, log)"
                      >
                        <Icon name="infoCircle" size="xs" />
                      </button>
                    </div>
                  </td>
                  <td>
                    <div class="usage-log-latency" data-testid="log-latency">
                      <div>
                        <span>{{ t('usage.analytics.firstTokenLatency') }}</span>
                        <strong>{{ formatOptionalLatency(log.first_token_ms) }}</strong>
                      </div>
                      <div :class="latencyTone(log.duration_ms)">
                        <span>{{ t('usage.analytics.totalLatency') }}</span>
                        <strong>{{ formatOptionalLatency(log.duration_ms) }}</strong>
                      </div>
                    </div>
                  </td>
                  <td><span class="usage-status"><i />200 OK</span></td>
                  <td class="usage-log-table__credits">
                    <div class="usage-credit-cell">
                      <span>{{ formatCreditValue(usdToCredits(log.actual_cost)) }}</span>
                      <button
                        type="button"
                        class="usage-credit-detail-trigger"
                        data-testid="credit-details-trigger"
                        :aria-label="t('usage.costDetails')"
                        @mouseenter="showCreditTooltip($event, log)"
                        @mouseleave="hideCreditTooltip"
                        @focus="showCreditTooltip($event, log)"
                        @blur="hideCreditTooltip"
                        @click.stop="toggleCreditTooltip($event, log)"
                      >
                        <Icon name="infoCircle" size="xs" />
                      </button>
                    </div>
                  </td>
                  <td class="usage-log-table__action">
                    <button
                      type="button"
                      class="usage-request-details-trigger"
                      data-testid="request-details-trigger"
                      :aria-label="t('usage.requestDetails.openAria')"
                      @click="openRequestDetails(log)"
                    >
                      <Icon name="eye" size="xs" />
                      <span>{{ t('usage.requestDetails.action') }}</span>
                    </button>
                  </td>
                </tr>
                <tr v-if="!loading && filteredUsageLogs.length === 0">
                  <td colspan="8" class="usage-log-table__empty">{{ t('usage.noRecords') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="pagination.total > 0" class="usage-log-panel__pagination">
            <span>{{ t('usage.analytics.recordCount', { count: formatNumber(pagination.total) }) }}</span>
            <Pagination
              :page="pagination.page"
              :total="pagination.total"
              :page-size="pagination.page_size"
              @update:page="handlePageChange"
              @update:pageSize="handlePageSizeChange"
            />
          </div>
        </template>

        <UserErrorRequestsTable
          v-else-if="errorViewEnabled"
          :rows="errorRows"
          :total="errorTotal"
          :loading="errorLoading"
          :page="errorPage"
          :page-size="errorPageSize"
          :visible-column-keys="errVisibleColumnKeys"
          @sort="onErrorSort"
          @update:page="onErrorPage"
          @update:pageSize="onErrorPageSize"
          @ipGeoBatchFailed="handleIpGeoBatchFailed"
        />
      </section>
    </div>
  </AppLayout>

  <UsageRequestDetailsDialog :log="requestDetailsLog" @close="requestDetailsLog = null" />

  <Teleport to="body">
    <div
      v-if="tokenTooltipVisible && tokenTooltipData"
      class="usage-token-tooltip"
      :style="{ left: `${tokenTooltipPosition.x}px`, top: `${tokenTooltipPosition.y}px` }"
      role="tooltip"
    >
      <strong>{{ t('usage.tokenDetails') }}</strong>
      <dl>
        <div><dt>{{ t('admin.usage.inputTokens') }}</dt><dd>{{ formatNumber(tokenTooltipData.input_tokens) }}</dd></div>
        <div><dt>{{ t('admin.usage.outputTokens') }}</dt><dd>{{ formatNumber(tokenTooltipData.output_tokens) }}</dd></div>
        <div><dt>{{ t('admin.usage.cacheCreationTokens') }}</dt><dd>{{ formatNumber(tokenTooltipData.cache_creation_tokens) }}</dd></div>
        <div><dt>{{ t('admin.usage.cacheReadTokens') }}</dt><dd>{{ formatNumber(tokenTooltipData.cache_read_tokens) }}</dd></div>
        <div class="usage-token-tooltip__total"><dt>{{ t('usage.totalTokens') }}</dt><dd>{{ formatNumber(totalLogTokens(tokenTooltipData)) }}</dd></div>
      </dl>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="creditTooltipVisible && creditTooltipData"
      class="usage-credit-tooltip"
      :style="{ left: `${creditTooltipPosition.x}px`, top: `${creditTooltipPosition.y}px` }"
      role="tooltip"
    >
      <strong>{{ t('usage.costDetails') }}</strong>
      <dl>
        <div><dt>{{ t('admin.usage.inputCost') }}</dt><dd>{{ formatCreditDetail(creditTooltipData.input_cost) }}</dd></div>
        <div><dt>{{ t('admin.usage.outputCost') }}</dt><dd>{{ formatCreditDetail(creditTooltipData.output_cost) }}</dd></div>
        <div><dt>{{ t('admin.usage.cacheCreationCost') }}</dt><dd>{{ formatCreditDetail(creditTooltipData.cache_creation_cost) }}</dd></div>
        <div><dt>{{ t('admin.usage.cacheReadCost') }}</dt><dd>{{ formatCreditDetail(creditTooltipData.cache_read_cost) }}</dd></div>
        <div class="usage-credit-tooltip__divider"><dt>{{ t('usage.inputTokenPrice') }}</dt><dd>{{ formatCreditUnitPrice(creditTooltipData.input_cost, creditTooltipData.input_tokens) }}</dd></div>
        <div><dt>{{ t('usage.outputTokenPrice') }}</dt><dd>{{ formatCreditUnitPrice(creditTooltipData.output_cost, creditTooltipData.output_tokens) }}</dd></div>
        <div class="usage-credit-tooltip__divider"><dt>{{ t('usage.serviceTier') }}</dt><dd>{{ getUsageServiceTierLabel(creditTooltipData.service_tier, t) }}</dd></div>
        <div><dt>{{ t('usage.rate') }}</dt><dd>{{ formatRateMultiplier(creditTooltipData.rate_multiplier) }}x</dd></div>
      </dl>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { keysAPI, usageAPI, userGroupsAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Icon from '@/components/icons/Icon.vue'
import UserErrorRequestsTable from '@/components/user/UserErrorRequestsTable.vue'
import UsageLatencyHeatmap from '@/components/user/usage/UsageLatencyHeatmap.vue'
import UsageModelCreditChart from '@/components/user/usage/UsageModelCreditChart.vue'
import UsageRequestDetailsDialog from '@/components/user/usage/UsageRequestDetailsDialog.vue'
import UsageTokenBarChart from '@/components/user/usage/UsageTokenBarChart.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatReasoningEffort } from '@/utils/format'
import { getBillingModeLabel, getDisplayBillingMode } from '@/utils/billingMode'
import { formatCredits, usdToCredits } from '@/utils/credit'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType, requestTypeToLegacyStream } from '@/utils/usageRequestType'
import type {
  ApiKey,
  Group,
  ModelStat,
  TrendDataPoint,
  UsageLog,
  UsageQueryParams,
  UsageStatsResponse,
  UserErrorRequest,
} from '@/types'
import type { UsagePreviewData } from '@/mocks/usagePreview'
import { COMMON_ERROR_STATUS_CODES } from '@/utils/errorBadges'

const { t } = useI18n()
const appStore = useAppStore()

const props = withDefaults(defineProps<{
  previewData?: UsagePreviewData
}>(), {
  previewData: undefined,
})

const previewMode = import.meta.env.DEV && Boolean(props.previewData)

const usageStats = ref<UsageStatsResponse | null>(null)
const monthToDateActualCost = ref(0)
const usageLogs = ref<UsageLog[]>([])
const analyticsLogs = ref<UsageLog[]>([])
const previewSourceLogs = ref<UsageLog[]>([])
const trendData = ref<TrendDataPoint[]>([])
const requestedModelStats = ref<ModelStat[]>([])

const loading = ref(false)
const chartsLoading = ref(false)
const modelStatsLoading = ref(false)
const exporting = ref(false)
const showAdvancedFilters = ref(false)
const logSearch = ref('')
const tokenTooltipVisible = ref(false)
const tokenTooltipPinned = ref(false)
const tokenTooltipData = ref<UsageLog | null>(null)
const tokenTooltipPosition = reactive({ x: 0, y: 0 })
const creditTooltipVisible = ref(false)
const creditTooltipPinned = ref(false)
const creditTooltipData = ref<UsageLog | null>(null)
const creditTooltipPosition = reactive({ x: 0, y: 0 })
const requestDetailsLog = ref<UsageLog | null>(null)
const errorRows = ref<UserErrorRequest[]>([])
const errorLoading = ref(false)
const errorPage = ref(1)
const errorPageSize = ref(20)
const errorSortBy = ref('created_at')
const errorSortOrder = ref<'asc' | 'desc'>('desc')
const errorTotal = ref(0)
const errorFilter = ref<{ model: string | null; category: string; api_key_id: number | null; status_code: number | null }>({
  model: '',
  category: '',
  api_key_id: null,
  status_code: null,
})

const errorKeyOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allKeys') },
  ...apiKeys.value.map((k) => ({ value: k.id, label: k.name })),
])

// 模型候选取自当前已加载错误中出现过的模型；creatable 允许输入任意片段做后端模糊。
const errorModelOptions = computed<SelectOption[]>(() => {
  const seen = new Set<string>()
  const opts: SelectOption[] = []
  for (const r of errorRows.value) {
    if (r.model && !seen.has(r.model)) {
      seen.add(r.model)
      opts.push({ value: r.model, label: r.model })
    }
  }
  return opts
})

const errorCategoryCodes = ['auth', 'rate_limit', 'quota', 'invalid_request', 'service_unavailable', 'upstream', 'internal', 'cyber']

const errorCategoryOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('usage.errors.allCategories') },
  ...errorCategoryCodes.map((c) => ({ value: c, label: t('usage.errors.categories.' + c) })),
])

// 状态码候选用固定常用列表(与管理端 UsageFilters 共用常量),不受当前页数据限制:
// 后端 status_code 过滤对全量生效,若只列当前页出现过的码,用户就选不到仅在后续页的码。
const errorStatusOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allStatuses') },
  ...COMMON_ERROR_STATUS_CODES.map((c) => ({ value: c, label: String(c) })),
])

const applyErrorFilters = () => {
  errorPage.value = 1
  void loadErrors()
}

let abortController: AbortController | null = null
let chartReqSeq = 0
let statsReqSeq = 0
let modelStatsReqSeq = 0

const formatLocalDate = (date: Date): string =>
  `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`

const getRangeForDays = (days: number) => {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - Math.max(0, days - 1))
  return { start: formatLocalDate(start), end: formatLocalDate(end) }
}

const getGranularityForRange = (start: string, end: string): 'day' | 'hour' => {
  const startTime = new Date(`${start}T00:00:00`).getTime()
  const endTime = new Date(`${end}T00:00:00`).getTime()
  return Math.ceil((endTime - startTime) / (1000 * 60 * 60 * 24)) <= 1 ? 'hour' : 'day'
}

const defaultRange = getRangeForDays(7)
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)
const granularity = ref<'day' | 'hour'>(getGranularityForRange(startDate.value, endDate.value))

const activeTab = ref<'usage' | 'errors'>('usage')
const errorViewEnabled = computed(() => previewMode
  ? false
  : appStore.cachedPublicSettings?.allow_user_view_error_requests ?? false)

const activeRangePreset = computed<'7' | '30' | 'custom'>(() => {
  const start = new Date(`${startDate.value}T00:00:00`).getTime()
  const end = new Date(`${endDate.value}T00:00:00`).getTime()
  const days = Math.round((end - start) / 86_400_000) + 1
  if (days === 7) return '7'
  if (days === 30) return '30'
  return 'custom'
})

const totalActualCredits = computed(() => usdToCredits(usageStats.value?.total_actual_cost))
const averageLatency = computed(() => Number(usageStats.value?.average_duration_ms || 0))
const projectedPeriodDays = computed(() => activeRangePreset.value === '7' ? 7 : 30)
const projectedPeriodCredits = computed(() => {
  const now = new Date()
  const elapsedDays = Math.max(1, now.getDate())
  return (usdToCredits(monthToDateActualCost.value) / elapsedDays) * projectedPeriodDays.value
})

const filteredUsageLogs = computed(() => {
  const query = logSearch.value.trim().toLowerCase()
  if (!query) return usageLogs.value
  return usageLogs.value.filter((log) =>
    log.model.toLowerCase().includes(query)
    || log.request_id.toLowerCase().includes(query)
    || (log.api_key?.name || '').toLowerCase().includes(query))
})

const formatNumber = (value: number) => new Intl.NumberFormat().format(value)
const formatCompactNumber = (value: number) => new Intl.NumberFormat(undefined, {
  notation: 'compact',
  maximumFractionDigits: 1,
}).format(value)
const formatCreditValue = (value: number) => `${formatCredits(value)} Credits`
const formatCreditDetail = (usd: number) => `${formatCredits(usdToCredits(usd), 6)} Credits`
const formatCreditUnitPrice = (usd: number, tokens: number) => {
  if (tokens <= 0) return `0 ${t('usage.perMillionTokens').trim()}`
  const creditsPerMillion = usdToCredits(usd) / tokens * 1_000_000
  return `${formatCredits(creditsPerMillion, 2)} Credits ${t('usage.perMillionTokens').trim()}`
}
const formatLatency = (value: number) => value >= 1000
  ? `${(value / 1000).toFixed(2)}s`
  : `${Math.round(value)}ms`
const formatOptionalLatency = (value: number | null | undefined) => value == null ? '--' : formatLatency(value)
const formatLogDate = (value: string) => new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
}).format(new Date(value))
const formatLogTime = (value: string) => new Intl.DateTimeFormat(undefined, {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
}).format(new Date(value))
const totalLogTokens = (log: UsageLog) =>
  log.input_tokens + log.output_tokens + log.cache_creation_tokens + log.cache_read_tokens
const formatRateMultiplier = (value: number) => Number(value || 1)
  .toFixed(2)
  .replace(/\.00$/, '')
  .replace(/(\.\d)0$/, '$1')
const formatLogGroup = (log: UsageLog) =>
  `${log.group?.name || t('usage.analytics.unknownGroup')} (${formatRateMultiplier(log.rate_multiplier)}x)`
// 延迟分档：<5s 绿、5~10s 黄、>10s 红。
// 阈值以秒为单位，因为网关请求本来就在秒级，毫秒级的档位分不出有效信息。
const LATENCY_SLOW_MS = 5000
const LATENCY_CRITICAL_MS = 10000

const latencyTone = (duration: number | null) => {
  const ms = Number(duration || 0)
  return {
    'usage-latency--fast': ms < LATENCY_SLOW_MS,
    'usage-latency--slow': ms >= LATENCY_SLOW_MS && ms <= LATENCY_CRITICAL_MS,
    'usage-latency--critical': ms > LATENCY_CRITICAL_MS,
  }
}

const positionTokenTooltip = (target: EventTarget | null) => {
  const element = target as HTMLElement | null
  if (!element) return
  const rect = element.getBoundingClientRect()
  const tooltipWidth = 230
  const preferredX = rect.right + 10
  tokenTooltipPosition.x = preferredX + tooltipWidth <= window.innerWidth
    ? preferredX
    : Math.max(8, rect.left - tooltipWidth - 10)
  tokenTooltipPosition.y = Math.min(window.innerHeight - 100, Math.max(100, rect.top + rect.height / 2))
}

const showTokenTooltip = (event: MouseEvent | FocusEvent, log: UsageLog) => {
  if (tokenTooltipPinned.value && tokenTooltipData.value?.id !== log.id) return
  closePinnedCreditTooltip()
  tokenTooltipData.value = log
  positionTokenTooltip(event.currentTarget)
  tokenTooltipVisible.value = true
}

const hideTokenTooltip = () => {
  if (!tokenTooltipPinned.value) tokenTooltipVisible.value = false
}

const toggleTokenTooltip = (event: MouseEvent, log: UsageLog) => {
  const closing = tokenTooltipPinned.value && tokenTooltipData.value?.id === log.id
  tokenTooltipPinned.value = !closing
  tokenTooltipVisible.value = !closing
  if (!closing) {
    tokenTooltipData.value = log
    positionTokenTooltip(event.currentTarget)
  }
}

const closePinnedTokenTooltip = () => {
  tokenTooltipPinned.value = false
  tokenTooltipVisible.value = false
}

const positionCreditTooltip = (target: EventTarget | null) => {
  const element = target as HTMLElement | null
  if (!element) return
  const rect = element.getBoundingClientRect()
  const tooltipWidth = 280
  const preferredX = rect.right + 10
  creditTooltipPosition.x = preferredX + tooltipWidth <= window.innerWidth
    ? preferredX
    : Math.max(8, rect.left - tooltipWidth - 10)
  creditTooltipPosition.y = Math.min(window.innerHeight - 150, Math.max(150, rect.top + rect.height / 2))
}

const showCreditTooltip = (event: MouseEvent | FocusEvent, log: UsageLog) => {
  if (creditTooltipPinned.value && creditTooltipData.value?.id !== log.id) return
  closePinnedTokenTooltip()
  creditTooltipData.value = log
  positionCreditTooltip(event.currentTarget)
  creditTooltipVisible.value = true
}

const hideCreditTooltip = () => {
  if (!creditTooltipPinned.value) creditTooltipVisible.value = false
}

const toggleCreditTooltip = (event: MouseEvent, log: UsageLog) => {
  const closing = creditTooltipPinned.value && creditTooltipData.value?.id === log.id
  creditTooltipPinned.value = !closing
  creditTooltipVisible.value = !closing
  if (!closing) {
    closePinnedTokenTooltip()
    creditTooltipData.value = log
    positionCreditTooltip(event.currentTarget)
  }
}

const closePinnedCreditTooltip = () => {
  creditTooltipPinned.value = false
  creditTooltipVisible.value = false
}

const closePinnedDetails = () => {
  closePinnedTokenTooltip()
  closePinnedCreditTooltip()
}

const openRequestDetails = (log: UsageLog) => {
  closePinnedDetails()
  requestDetailsLog.value = log
}

const filters = ref<UsageQueryParams>({
  start_date: startDate.value,
  end_date: endDate.value,
  request_type: undefined,
  billing_type: null,
  billing_mode: null,
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
})
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc',
})

const requestTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allTypes') },
  { value: 'ws_v2', label: t('usage.ws') },
  { value: 'stream', label: t('usage.stream') },
  { value: 'sync', label: t('usage.sync') },
])
const billingTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingTypes') },
  { value: 0, label: t('admin.usage.billingTypeBalance') },
  { value: 1, label: t('admin.usage.billingTypeSubscription') },
])
const billingModeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingModes') },
  { value: 'token', label: t('admin.usage.billingModeToken') },
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') },
  { value: 'image', label: t('admin.usage.billingModeImage') },
  { value: 'video', label: t('admin.usage.billingModeVideo') },
])

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
const modelOptionValues = ref<string[]>([])

const apiKeyOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.allApiKeys') },
  ...apiKeys.value.map((key) => ({ value: key.id, label: key.name })),
])
const groupOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allGroups') },
  ...groups.value.map((group) => ({ value: group.id, label: group.name })),
])
const modelOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allModels') },
  ...modelOptionValues.value.map((model) => ({ value: model, label: model })),
])

const normalizedFilters = computed<UsageQueryParams>(() => {
  const requestType = filters.value.request_type
  const legacyStream = requestType ? requestTypeToLegacyStream(requestType) : filters.value.stream
  return {
    ...filters.value,
    start_date: startDate.value,
    end_date: endDate.value,
    stream: legacyStream === null ? undefined : legacyStream,
  }
})

const buildUsageListParams = (page: number, pageSize: number): UsageQueryParams => ({
  page,
  page_size: pageSize,
  ...normalizedFilters.value,
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order,
})

const getFilteredPreviewLogs = () => previewSourceLogs.value.filter((log) => {
  const filter = normalizedFilters.value
  const logDate = formatLocalDate(new Date(log.created_at))
  if (filter.start_date && logDate < filter.start_date) return false
  if (filter.end_date && logDate > filter.end_date) return false
  if (filter.api_key_id != null && log.api_key_id !== filter.api_key_id) return false
  if (filter.group_id != null && log.group_id !== filter.group_id) return false
  if (filter.model && log.model !== filter.model) return false
  if (filter.billing_type != null && log.billing_type !== filter.billing_type) return false
  if (filter.billing_mode && getDisplayBillingMode(log) !== filter.billing_mode) return false
  if (filter.request_type && resolveUsageRequestType(log) !== filter.request_type) return false
  return true
})

const loadLogs = async () => {
  if (previewMode) {
    const filtered = getFilteredPreviewLogs()
    const offset = (pagination.page - 1) * pagination.page_size
    usageLogs.value = filtered.slice(offset, offset + pagination.page_size)
    analyticsLogs.value = filtered
    pagination.total = filtered.length
    return
  }
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    const res = await usageAPI.query(buildUsageListParams(pagination.page, pagination.page_size), {
      signal: controller.signal,
    })
    if (!controller.signal.aborted) {
      usageLogs.value = res.items
      analyticsLogs.value = res.items
      pagination.total = res.total
    }
  } catch (error: any) {
    if (error?.name !== 'AbortError' && error?.code !== 'ERR_CANCELED') {
      appStore.showError(t('usage.failedToLoad'))
    }
  } finally {
    if (abortController === controller) loading.value = false
  }
}

const loadStats = async () => {
  if (previewMode) {
    usageStats.value = props.previewData?.stats || null
    monthToDateActualCost.value = props.previewData?.monthToDateActualCost
      ?? props.previewData?.stats.total_actual_cost
      ?? 0
    return
  }
  const seq = ++statsReqSeq
  try {
    const now = new Date()
    const monthStart = new Date(now.getFullYear(), now.getMonth(), 1)
    const monthFilters: UsageQueryParams = {
      ...normalizedFilters.value,
      start_date: formatLocalDate(monthStart),
      end_date: formatLocalDate(now),
    }
    const [stats, monthStats] = await Promise.all([
      usageAPI.getStats(normalizedFilters.value),
      usageAPI.getStats(monthFilters),
    ])
    if (seq !== statsReqSeq) return
    usageStats.value = stats
    monthToDateActualCost.value = monthStats.total_actual_cost
  } catch (error) {
    if (seq !== statsReqSeq) return
    console.error('Failed to load usage stats:', error)
  }
}

const loadModelStats = async () => {
  if (previewMode) {
    requestedModelStats.value = props.previewData?.models || []
    refreshModelOptions(requestedModelStats.value)
    return
  }
  const seq = ++modelStatsReqSeq
  modelStatsLoading.value = true
  try {
    const response = await usageAPI.getDashboardModels({
      ...normalizedFilters.value,
      model_source: 'requested',
    })
    if (seq !== modelStatsReqSeq) return
    requestedModelStats.value = response.models || []
    refreshModelOptions(response.models || [])
  } catch (error) {
    if (seq !== modelStatsReqSeq) return
    console.error('Failed to load model stats:', error)
    requestedModelStats.value = []
  } finally {
    if (seq === modelStatsReqSeq) modelStatsLoading.value = false
  }
}

const loadChartData = async () => {
  if (previewMode) {
    trendData.value = props.previewData?.trend || []
    return
  }
  const seq = ++chartReqSeq
  chartsLoading.value = true
  try {
    const snapshot = await usageAPI.getDashboardSnapshotV2({
      ...normalizedFilters.value,
      granularity: granularity.value,
      include_trend: true,
      include_model_stats: false,
      include_group_stats: true,
    })
    if (seq !== chartReqSeq) return
    trendData.value = snapshot.trend || []
  } catch (error) {
    if (seq !== chartReqSeq) return
    console.error('Failed to load chart data:', error)
    trendData.value = []
  } finally {
    if (seq === chartReqSeq) chartsLoading.value = false
  }
}

const refreshModelOptions = (models: ModelStat[]) => {
  const current = filters.value.model
  const set = new Set(modelOptionValues.value)
  models.forEach((item) => {
    if (item.model) set.add(item.model)
  })
  if (current) set.add(current)
  modelOptionValues.value = Array.from(set).sort()
}

const applyFilters = () => {
  pagination.page = 1
  void loadLogs()
  void loadStats()
  void loadModelStats()
  void loadChartData()
  resetErrorRows()
}

const refreshData = () => {
  void loadLogs()
  void loadStats()
  void loadModelStats()
  void loadChartData()
  if (activeTab.value === 'errors') void loadErrors()
}

const resetFilters = () => {
  const range = getRangeForDays(7)
  startDate.value = range.start
  endDate.value = range.end
  filters.value = {
    start_date: range.start,
    end_date: range.end,
    request_type: undefined,
    billing_type: null,
    billing_mode: null,
  }
  granularity.value = getGranularityForRange(range.start, range.end)
  applyFilters()
  if (activeTab.value === 'errors') {
    errorFilter.value = { model: '', category: '', api_key_id: null, status_code: null }
    applyErrorFilters()
  }
}

const applyDatePreset = (days: number) => {
  const range = getRangeForDays(days)
  startDate.value = range.start
  endDate.value = range.end
  filters.value.start_date = range.start
  filters.value.end_date = range.end
  granularity.value = getGranularityForRange(range.start, range.end)
  applyFilters()
}

const onDateRangeChange = (range: { startDate: string; endDate: string; preset: string | null }) => {
  startDate.value = range.startDate
  endDate.value = range.endDate
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  granularity.value = getGranularityForRange(range.startDate, range.endDate)
  applyFilters()
}

const handlePageChange = (page: number) => {
  pagination.page = page
  void loadLogs()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadLogs()
}

const handleIpGeoBatchFailed = () => {
  appStore.showError(t('usage.ipGeo.batchFailed'))
}

const getRequestTypeExportText = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'cyber') return 'Cyber'
  if (requestType === 'ws_v2') return 'WS'
  if (requestType === 'stream') return 'Stream'
  if (requestType === 'sync') return 'Sync'
  return 'Unknown'
}

const escapeCSVValue = (value: unknown): string => {
  if (value == null) return ''
  const str = String(value)
  const escaped = str.replace(/"/g, '""')
  if (/^[=+\-@\t\r]/.test(str)) return `"\'${escaped}"`
  if (/[,"\n\r]/.test(str)) return `"${escaped}"`
  return str
}

const exportToCSV = async () => {
  if (pagination.total === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  }
  exporting.value = true
  appStore.showInfo(t('usage.preparingExport'))
  try {
    const allLogs: UsageLog[] = []
    if (previewMode) {
      allLogs.push(...getFilteredPreviewLogs())
    } else {
      const pageSize = 100
      const totalPages = Math.ceil(pagination.total / pageSize)
      for (let page = 1; page <= totalPages; page++) {
        const response = await usageAPI.query(buildUsageListParams(page, pageSize))
        allLogs.push(...response.items)
      }
    }
    if (allLogs.length === 0) {
      appStore.showWarning(t('usage.noDataToExport'))
      return
    }
    const headers = [
      'Time',
      'API Key Name',
      'Model',
      'Reasoning Effort',
      'Inbound Endpoint',
      'IP Address',
      'Type',
      'Billing Mode',
      'Input Tokens',
      'Output Tokens',
      'Cache Read Tokens',
      'Cache Creation Tokens',
      'Rate Multiplier',
      'Actual Credits',
      'Standard Credits',
      'First Token (ms)',
      'Duration (ms)',
    ]
    const rows = allLogs.map((log) => [
      log.created_at,
      log.api_key?.name || '',
      log.model,
      formatReasoningEffort(log.reasoning_effort),
      log.inbound_endpoint || '',
      log.ip_address || '',
      getRequestTypeExportText(log),
      getBillingModeLabel(getDisplayBillingMode(log), t),
      log.input_tokens,
      log.output_tokens,
      log.cache_read_tokens,
      log.cache_creation_tokens,
      log.rate_multiplier,
      usdToCredits(log.actual_cost).toFixed(4),
      usdToCredits(log.total_cost).toFixed(4),
      log.first_token_ms ?? '',
      log.duration_ms ?? '',
    ].map(escapeCSVValue))
    const csvContent = [
      headers.map(escapeCSVValue).join(','),
      ...rows.map((row) => row.join(',')),
    ].join('\n')
    const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `usage_${startDate.value}_to_${endDate.value}.csv`
    link.click()
    window.URL.revokeObjectURL(url)
    appStore.showSuccess(t('usage.exportSuccess'))
  } catch (error) {
    console.error('CSV Export failed:', error)
    appStore.showError(t('usage.exportFailed'))
  } finally {
    exporting.value = false
  }
}

const errVisibleColumnKeys = computed(() => [
  'key_name', 'model', 'endpoint', 'client_ip', 'group', 'type',
  'platform', 'category', 'status', 'message', 'created_at',
])

const loadFilterOptions = async () => {
  if (previewMode) {
    apiKeys.value = props.previewData?.apiKeys || []
    groups.value = props.previewData?.groups || []
    return
  }
  try {
    const [keys, availableGroups] = await Promise.all([
      keysAPI.list(1, 100),
      userGroupsAPI.getAvailable(),
    ])
    apiKeys.value = keys.items
    groups.value = availableGroups
  } catch (error) {
    console.error('Failed to load usage filter options:', error)
  }
}

const resetErrorRows = () => {
  errorPage.value = 1
  if (activeTab.value === 'errors') {
    void loadErrors()
  } else {
    errorRows.value = []
    errorTotal.value = 0
  }
}

const loadErrors = async () => {
  errorLoading.value = true
  try {
    const resp = await usageAPI.listMyErrorRequests({
      page: errorPage.value,
      page_size: errorPageSize.value,
      start_date: startDate.value,
      end_date: endDate.value,
      model: (errorFilter.value.model ?? '').trim() || undefined,
      category: errorFilter.value.category || undefined,
      api_key_id: errorFilter.value.api_key_id ?? undefined,
      status_code: errorFilter.value.status_code ?? undefined,
      sort_by: errorSortBy.value,
      sort_order: errorSortOrder.value,
    })
    errorRows.value = resp.items
    errorTotal.value = resp.total
  } catch (error) {
    console.error('[UsageView] loadErrors failed:', error)
    appStore.showError(t('usage.errors.failedToLoad'))
  } finally {
    errorLoading.value = false
  }
}

const onErrorSort = (sortBy: string, sortOrder: 'asc' | 'desc') => {
  errorSortBy.value = sortBy
  errorSortOrder.value = sortOrder
  errorPage.value = 1
  void loadErrors()
}

const onErrorPage = (page: number) => {
  errorPage.value = page
  void loadErrors()
}

const onErrorPageSize = (pageSize: number) => {
  errorPageSize.value = pageSize
  errorPage.value = 1
  void loadErrors()
}

const switchToErrors = () => {
  activeTab.value = 'errors'
  if (errorRows.value.length === 0) void loadErrors()
}

onMounted(() => {
  if (previewMode) {
    previewSourceLogs.value = props.previewData?.logs || []
  }
  void loadFilterOptions()
  refreshData()
  document.addEventListener('click', closePinnedDetails)
})

onUnmounted(() => {
  abortController?.abort()
  document.removeEventListener('click', closePinnedDetails)
})
</script>

<style scoped>
.usage-analytics {
  --usage-panel: var(--md-surface-container-low);
  --usage-panel-deep: var(--md-app-bg);
  --usage-border: var(--md-outline-variant);
  --usage-border-soft: rgba(72, 96, 89, .46);
  --usage-text: var(--md-on-surface);
  --usage-muted: var(--md-on-surface-variant);
  --usage-accent: #00f5a8;
  position: relative;
  min-height: calc(100vh - 96px);
  color: var(--usage-text);
}

.usage-analytics__hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 18px;
}

.usage-analytics__hero h1 {
  margin: 8px 0 6px;
  font-size: clamp(28px, 4vw, 44px);
  line-height: 1.05;
  font-weight: 800;
}

.usage-analytics__hero p { color: var(--usage-muted); font-size: 13px; }
.usage-analytics__live { display: inline-flex; align-items: center; gap: 7px; color: var(--usage-accent); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-analytics__live span { width: 7px; height: 7px; border-radius: 50%; background: var(--usage-accent); box-shadow: 0 0 12px rgba(0, 245, 168, .7); animation: usage-pulse 2s infinite; }

.usage-range-control {
  display: flex;
  min-height: 38px;
  align-items: stretch;
  border: 1px solid var(--usage-border);
  background: rgba(7, 15, 18, .82);
}

.usage-range-control > button {
  min-width: 88px;
  padding: 8px 13px;
  border-right: 1px solid var(--usage-border);
  color: var(--md-on-surface-variant);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  transition: color .15s ease, background-color .15s ease;
}

.usage-range-control > button:hover,
.usage-range-control > button.active { background: var(--md-surface-container-low); color: var(--usage-text); }
.usage-range-control__custom { display: flex; align-items: center; padding: 3px; }
.usage-range-control__custom.active { background: var(--md-surface-container-low); }

.usage-credit-rule {
  display: flex;
  align-items: center;
  gap: 9px;
  width: fit-content;
  margin-bottom: 14px;
  border-left: 2px solid var(--usage-accent);
  background: rgba(0, 245, 168, .07);
  padding: 8px 11px;
  color: var(--md-on-surface-variant);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.usage-credit-rule :deep(svg) { color: var(--usage-accent); }
.usage-kpi-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; margin-bottom: 18px; }
.usage-kpi-card { min-height: 132px; border: 1px solid var(--usage-border); background: rgba(4, 10, 13, .88); padding: 20px 22px; box-shadow: inset 0 1px rgba(255,255,255,.015); }
.usage-kpi-card--primary { border-top-color: var(--usage-accent); }
.usage-kpi-card--warning { border-left: 3px solid #ff9f97; }
.usage-kpi-card__label { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--md-on-surface-variant); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; text-transform: uppercase; }
.usage-kpi-card__label :deep(svg) { color: var(--md-on-surface-variant); }
.usage-kpi-card strong { display: block; margin-top: 13px; overflow-wrap: anywhere; color: var(--usage-text); font-size: clamp(22px, 2.6vw, 31px); line-height: 1.1; }
.usage-kpi-card p { margin-top: 10px; color: var(--md-on-surface-variant); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-kpi-card--primary p { color: var(--usage-accent); }

.usage-chart-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr); gap: 18px; margin-bottom: 18px; }
:deep(.usage-panel) { min-width: 0; overflow: hidden; border: 1px solid var(--usage-border); background: rgba(17, 27, 31, .94); border-radius: 4px; box-shadow: 0 12px 35px rgba(0, 0, 0, .14); }
:deep(.usage-panel__header) { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 19px 20px 13px; }
:deep(.usage-panel__header h2),
.usage-log-panel__topbar h2 { color: var(--usage-text); font-size: 18px; line-height: 1.25; font-weight: 700; }
:deep(.usage-panel__eyebrow),
.usage-panel__eyebrow { margin-bottom: 5px; color: var(--usage-accent); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }

.usage-log-panel { margin-top: 18px; background: rgba(4, 10, 13, .96) !important; }
.usage-log-panel__topbar { display: flex; align-items: center; justify-content: space-between; gap: 18px; border-bottom: 1px solid var(--usage-border); padding: 17px 18px; }
.usage-log-actions { display: flex; align-items: stretch; gap: 7px; }
.usage-search { display: flex; width: min(290px, 32vw); align-items: center; gap: 8px; border: 1px solid var(--usage-border); background: var(--md-surface-container-low); padding: 0 10px; color: var(--md-on-surface-variant); }
.usage-search input { min-width: 0; width: 100%; height: 34px; border: 0; outline: 0; background: transparent; color: var(--usage-text); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-search input::placeholder { color: var(--md-on-surface-variant); }
.usage-icon-button { display: grid; width: 36px; height: 36px; place-items: center; border: 1px solid var(--usage-border); background: var(--md-surface-container-low); color: var(--md-on-surface-variant); transition: border-color .15s ease, color .15s ease, background-color .15s ease; }
.usage-icon-button:hover, .usage-icon-button.active { border-color: var(--usage-accent); background: rgba(0, 245, 168, .07); color: var(--usage-accent); }
.usage-icon-button:disabled { cursor: wait; opacity: .5; }

.usage-tabs { display: flex; gap: 18px; border-bottom: 1px solid var(--usage-border-soft); padding: 0 18px; }
.usage-tabs button { position: relative; padding: 11px 1px; color: var(--md-on-surface-variant); font-size: 12px; }
.usage-tabs button.active { color: var(--usage-text); }
.usage-tabs button.active::after { position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--usage-accent); content: ''; }

.usage-filter-panel { display: grid; grid-template-columns: repeat(6, minmax(140px, 1fr)); gap: 12px; border-bottom: 1px solid var(--usage-border); background: rgba(14, 26, 29, .72); padding: 15px 18px; }
.usage-filter-panel > div { min-width: 0; }
.usage-filter-panel label { display: block; margin-bottom: 6px; color: var(--md-on-surface-variant); font-size: 12px; }
.usage-reset-button { align-self: end; min-height: 38px; border: 1px solid var(--usage-border); color: var(--md-on-surface-variant); font-size: 12px; }
.usage-reset-button:hover { border-color: var(--md-on-surface-variant); color: var(--usage-text); }

.usage-table-scroll { overflow-x: auto; }
.usage-log-table { width: 100%; min-width: 980px; border-collapse: collapse; }
.usage-log-table th { border-bottom: 1px solid var(--usage-border); background: var(--md-surface-container-low); padding: 12px 18px; color: var(--md-on-surface-variant); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; font-weight: 600; text-align: left; text-transform: uppercase; }
.usage-log-table td { border-bottom: 1px solid var(--usage-border-soft); padding: 13px 18px; color: var(--md-on-surface-variant); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; vertical-align: middle; }
.usage-log-table tbody tr { transition: background-color .14s ease; }
.usage-log-table tbody tr:hover { background: rgba(24, 40, 43, .58); }
.usage-log-table__time strong, .usage-log-table__time span { display: block; }
.usage-log-table__time strong { color: var(--md-on-surface); font-weight: 500; }
.usage-log-table__time span { margin-top: 3px; color: var(--md-on-surface-variant); }
.usage-model-pill { display: block; width: fit-content; max-width: 210px; overflow: hidden; border: 1px solid var(--md-outline-variant); background: var(--md-surface-container-low); padding: 4px 7px; color: #c4e8dd; text-overflow: ellipsis; white-space: nowrap; }
.usage-group-label { color: var(--md-on-surface-variant); white-space: nowrap; }
.usage-token-cell, .usage-credit-cell { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
.usage-token-detail-trigger, .usage-credit-detail-trigger { display: grid; width: 16px; height: 16px; place-items: center; color: var(--md-on-surface-variant); transition: color .14s ease; }
.usage-token-detail-trigger:hover, .usage-token-detail-trigger:focus-visible,
.usage-credit-detail-trigger:hover, .usage-credit-detail-trigger:focus-visible { color: var(--usage-accent); outline: none; }
.usage-log-table td small { display: block; max-width: 200px; margin-top: 4px; overflow: hidden; color: var(--md-on-surface-variant); text-overflow: ellipsis; white-space: nowrap; }
.usage-status { display: inline-flex; align-items: center; gap: 7px; color: var(--usage-accent); white-space: nowrap; }
.usage-status i { width: 6px; height: 6px; border-radius: 50%; background: var(--usage-accent); }
.usage-log-latency { display: grid; min-width: 116px; gap: 4px; white-space: nowrap; }
.usage-log-latency > div { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
.usage-log-latency span { color: var(--md-on-surface-variant); font-size: 12px; }
.usage-log-latency strong { color: var(--md-on-surface); font-size: 12px; font-weight: 600; }
.usage-log-latency .usage-latency--fast strong { color: var(--usage-accent); }
.usage-log-latency .usage-latency--slow strong { color: #ffbd59; }
.usage-log-latency .usage-latency--critical strong { color: #ff9f97; }
.usage-log-table__credits { color: var(--usage-accent) !important; font-weight: 700; white-space: nowrap; }
.usage-log-table__action-heading { width: 84px; text-align: right !important; }
.usage-log-table__action { text-align: right; }
.usage-request-details-trigger { display: inline-flex; min-width: 58px; height: 28px; align-items: center; justify-content: center; gap: 6px; border: 1px solid var(--md-outline-variant); background: var(--md-surface-container-low); padding: 0 9px; color: var(--md-on-surface-variant); font-family: inherit; font-size: 12px; transition: border-color .14s ease, background-color .14s ease, color .14s ease; }
.usage-request-details-trigger:hover, .usage-request-details-trigger:focus-visible { border-color: var(--usage-accent); outline: none; background: rgba(0, 245, 168, .07); color: var(--usage-accent); }
.usage-latency--fast { color: var(--usage-accent) !important; }
.usage-latency--slow { color: #ffbd59 !important; }
.usage-latency--critical { color: #ff9f97 !important; }
.usage-log-table__empty { height: 150px; color: var(--md-on-surface-variant) !important; text-align: center !important; }
.usage-log-panel__pagination { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 12px 18px; color: var(--md-on-surface-variant); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.usage-log-panel__pagination :deep(.card) { border: 0; background: transparent; box-shadow: none; }

@keyframes usage-pulse { 50% { opacity: .35; transform: scale(.86); } }

@media (max-width: 1180px) {
  .usage-filter-panel { grid-template-columns: repeat(3, minmax(150px, 1fr)); }
}

@media (max-width: 900px) {
  .usage-analytics__hero { align-items: flex-start; flex-direction: column; }
  .usage-range-control { width: 100%; overflow-x: auto; }
  .usage-range-control > button { flex: 1; }
  .usage-kpi-grid { grid-template-columns: 1fr; }
  .usage-kpi-card { min-height: 118px; }
  .usage-chart-grid { grid-template-columns: 1fr; }
}

@media (max-width: 640px) {
  .usage-analytics__hero h1 { font-size: 30px; }
  .usage-range-control { flex-wrap: wrap; }
  .usage-range-control > button { min-width: 50%; border-bottom: 1px solid var(--usage-border); }
  .usage-range-control__custom { width: 100%; }
  .usage-credit-rule { width: 100%; line-height: 1.5; }
  .usage-kpi-card { padding: 17px; }
  :deep(.usage-panel__header), .usage-log-panel__topbar { align-items: flex-start; flex-direction: column; }
  .usage-log-actions { width: 100%; }
  .usage-search { flex: 1; width: auto; }
  .usage-filter-panel { grid-template-columns: 1fr; }
  .usage-log-panel__pagination { align-items: flex-start; flex-direction: column; }
}
</style>

<style>
.usage-token-tooltip {
  position: fixed;
  z-index: 9999;
  width: 220px;
  transform: translateY(-50%);
  border: 1px solid var(--md-outline-variant);
  border-radius: 4px;
  background: var(--md-surface-container-low);
  padding: 12px 13px;
  color: var(--md-on-surface);
  box-shadow: 0 16px 42px rgba(0, 0, 0, .48);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  pointer-events: none;
}
.usage-token-tooltip > strong { display: block; margin-bottom: 8px; color: var(--md-on-surface); font-size: 12px; }
.usage-token-tooltip dl { display: grid; gap: 6px; }
.usage-token-tooltip dl > div { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.usage-token-tooltip dt { color: var(--md-on-surface-variant); }
.usage-token-tooltip dd { color: var(--md-on-surface); font-weight: 600; }
.usage-token-tooltip__total { margin-top: 3px; border-top: 1px solid var(--md-outline-variant); padding-top: 8px; }
.usage-token-tooltip__total dd { color: #00f5a8; }

.usage-credit-tooltip {
  position: fixed;
  z-index: 9999;
  width: 270px;
  transform: translateY(-50%);
  border: 1px solid var(--md-outline-variant);
  border-radius: 4px;
  background: var(--md-surface-container-low);
  padding: 12px 13px;
  color: var(--md-on-surface);
  box-shadow: 0 16px 42px rgba(0, 0, 0, .48);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  pointer-events: none;
}
.usage-credit-tooltip > strong { display: block; margin-bottom: 8px; color: var(--md-on-surface); font-size: 12px; }
.usage-credit-tooltip dl { display: grid; gap: 6px; }
.usage-credit-tooltip dl > div { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.usage-credit-tooltip dt { color: var(--md-on-surface-variant); }
.usage-credit-tooltip dd { color: var(--md-on-surface); font-weight: 600; text-align: right; white-space: nowrap; }
.usage-credit-tooltip__divider { margin-top: 3px; border-top: 1px solid var(--md-outline-variant); padding-top: 8px; }
.usage-credit-tooltip__divider dd { color: #00f5a8; }
</style>
