<template>
  <div
    v-if="hasBinding"
    data-testid="ye-team-refresh-state"
    class="mt-1 flex min-w-0 max-w-[320px] items-center gap-1.5"
    :title="title"
  >
    <button
      type="button"
      data-testid="ye-team-refresh-badge"
      class="inline-flex min-w-0 shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium leading-4 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40"
      :class="badgeClass"
      :aria-label="hasFlow ? t('admin.accounts.yeTeamRefresh.viewDetails') : label"
      :disabled="!hasFlow"
      @click="showDetails = true"
    >
      <span :class="['h-1.5 w-1.5 rounded-full', dotClass]" />
      <span class="truncate">{{ label }}</span>
      <Icon v-if="hasFlow" name="infoCircle" size="xs" class="shrink-0" />
    </button>
    <span
      v-if="state === 'success' && formattedAt"
      data-testid="ye-team-refresh-time"
      class="truncate text-[10px] leading-4 text-gray-500 dark:text-gray-400"
    >
      {{ formattedAt }}
    </span>
    <span
      v-else-if="state === 'failed' && lastError"
      data-testid="ye-team-refresh-error"
      class="truncate text-[10px] leading-4 text-red-600 dark:text-red-300"
    >
      {{ lastError }}
    </span>

    <BaseDialog
      :show="showDetails"
      :title="t('admin.accounts.yeTeamRefresh.detailsTitle')"
      width="wide"
      @close="showDetails = false"
    >
      <div v-if="flow" data-testid="ye-team-refresh-details" class="space-y-4">
        <section class="rounded-lg border border-gray-200 bg-gray-50/80 p-3 dark:border-dark-700 dark:bg-dark-800/60">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.accounts.yeTeamRefresh.overview') }}</h4>
            <span :class="['inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium', flowStatusClass]">
              <span :class="['h-1.5 w-1.5 rounded-full', flowDotClass]" />
              {{ flowStatusLabel }}
            </span>
          </div>
          <div class="grid grid-cols-1 gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4">
            <div v-for="item in overviewItems" :key="item.label" class="min-w-0">
              <div class="text-gray-500 dark:text-gray-400">{{ item.label }}</div>
              <div class="mt-0.5 break-words font-medium text-gray-800 dark:text-gray-100">{{ item.value }}</div>
            </div>
          </div>
        </section>

        <section v-if="flow.batch" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.accounts.yeTeamRefresh.batch') }}</h4>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
            <div v-for="item in batchItems" :key="item.key" class="rounded border border-gray-100 bg-gray-50 px-2 py-1.5 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ item.label }}</div>
              <div class="mt-0.5 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ item.value }}</div>
            </div>
          </div>
        </section>

        <section v-if="taskRows.length" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
          <h4 class="mb-3 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.accounts.yeTeamRefresh.task') }}</h4>
          <div class="space-y-3">
            <div v-for="(task, index) in taskRows" :key="`${task.order_no || task.resource_uid || task.status}-${index}`" class="rounded border border-gray-100 p-2.5 dark:border-dark-700">
              <div v-if="taskRows.length > 1" class="mb-2 text-[11px] font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">#{{ index + 1 }}</div>
              <div class="grid grid-cols-1 gap-x-4 gap-y-2 text-xs sm:grid-cols-2">
                <div v-for="item in taskItems(task)" :key="item.label" class="min-w-0">
                  <div class="text-gray-500 dark:text-gray-400">{{ item.label }}</div>
                  <div class="mt-0.5 break-words font-medium text-gray-800 dark:text-gray-100" :class="item.tone">{{ item.value }}</div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section>
          <h4 class="mb-3 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.accounts.yeTeamRefresh.timeline') }}</h4>
          <ol data-testid="ye-team-refresh-timeline" class="relative ml-1 border-l border-gray-200 pl-4 dark:border-dark-700">
            <li v-for="stage in flow.stages || []" :key="`${stage.name}-${stage.at}`" class="relative pb-4 last:pb-0">
              <span :class="['absolute -left-[21px] top-0.5 flex h-3 w-3 items-center justify-center rounded-full border-2 border-white dark:border-dark-900', stageDotClass(stage.status)]" />
              <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
                <span class="text-xs font-medium text-gray-800 dark:text-gray-100">{{ stageLabel(stage.name) }}</span>
                <span class="text-[10px] text-gray-500 dark:text-gray-400">{{ formatStageTime(stage.at) }}</span>
              </div>
              <div class="mt-0.5 flex flex-wrap items-center gap-2 text-[11px]">
                <span :class="stageTextClass(stage.status)">{{ stageStatusLabel(stage.status) }}</span>
                <span v-if="stage.message" class="break-words text-gray-600 dark:text-gray-300">{{ stage.message }}</span>
              </div>
            </li>
          </ol>
        </section>

        <section v-if="lastError" class="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-900/60 dark:bg-red-900/20">
          <h4 class="mb-1 text-sm font-semibold text-red-800 dark:text-red-200">{{ t('admin.accounts.yeTeamRefresh.errorDetails') }}</h4>
          <p data-testid="ye-team-refresh-details-error" class="break-words text-xs leading-5 text-red-700 dark:text-red-300">{{ lastError }}</p>
        </section>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import type { Account, YeTeamRefreshFlow, YeTeamRefreshStage } from '@/types'

type RefreshState = 'unrefreshed' | 'running' | 'success' | 'failed'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const showDetails = ref(false)

const extra = computed<Record<string, unknown>>(() => props.account.extra || {})
const hasBinding = computed(() => {
  const cardCode = extra.value.ye_team_card_code
  return typeof cardCode === 'string' && cardCode.trim().length > 0
})
const flow = computed<YeTeamRefreshFlow | null>(() => {
  const value = extra.value.ye_team_last_refresh_flow
  return value && typeof value === 'object' ? value as YeTeamRefreshFlow : null
})
const taskRows = computed(() => flow.value?.tasks?.length ? flow.value.tasks : flow.value?.task ? [flow.value.task] : [])
const hasFlow = computed(() => Boolean(flow.value?.stages?.length || flow.value?.batch || taskRows.value.length))
const state = computed<RefreshState>(() => {
  const flowState = flow.value?.status
  if (flowState === 'running' || flowState === 'success' || flowState === 'failed') return flowState
  const value = extra.value.ye_team_last_refresh_status
  if (value === 'success' || value === 'failed') return value
  return 'unrefreshed'
})
const lastError = computed(() => {
  const value = extra.value.ye_team_last_refresh_error
  return typeof value === 'string' ? value.trim() : ''
})
const formattedAt = computed(() => {
  const value = extra.value.ye_team_last_refresh_at
  return typeof value === 'string' ? formatDateTimeToMinute(value) : ''
})
const label = computed(() => t(`admin.accounts.yeTeamRefresh.${state.value === 'success' ? 'refreshed' : state.value === 'failed' ? 'failed' : state.value === 'running' ? 'running' : 'notRefreshed'}`))
const badgeClass = computed(() => {
  if (state.value === 'success') return 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-300 dark:hover:bg-emerald-900/50'
  if (state.value === 'failed') return 'bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-300 dark:hover:bg-red-900/50'
  if (state.value === 'running') return 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:hover:bg-blue-900/50'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})
const dotClass = computed(() => state.value === 'success' ? 'bg-emerald-500' : state.value === 'failed' ? 'bg-red-500' : state.value === 'running' ? 'bg-blue-500 animate-pulse' : 'bg-gray-400')
const title = computed(() => {
  const parts = [label.value]
  if (formattedAt.value) parts.push(formattedAt.value)
  if (state.value === 'failed' && lastError.value) parts.push(lastError.value)
  return parts.join(' · ')
})

const flowStatusLabel = computed(() => t(`admin.accounts.yeTeamRefresh.stageStatus.${flow.value?.status || state.value}`))
const flowStatusClass = computed(() => flow.value?.status === 'success' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : flow.value?.status === 'failed' ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300')
const flowDotClass = computed(() => flow.value?.status === 'success' ? 'bg-emerald-500' : flow.value?.status === 'failed' ? 'bg-red-500' : 'bg-blue-500 animate-pulse')
const formatStageTime = (value?: string) => value ? formatDateTimeToMinute(value) : '-'
const formatValue = (value: unknown) => value === undefined || value === null || value === '' ? '-' : String(value)
const duration = computed(() => {
  if (!flow.value?.started_at || !flow.value.finished_at) return '-'
  const elapsed = Date.parse(flow.value.finished_at) - Date.parse(flow.value.started_at)
  if (!Number.isFinite(elapsed) || elapsed < 0) return '-'
  if (elapsed < 1000) return `${elapsed} ms`
  return `${(elapsed / 1000).toFixed(1)} s`
})
const overviewItems = computed(() => [
  { label: t('admin.accounts.yeTeamRefresh.currentStatus'), value: flowStatusLabel.value },
  { label: t('admin.accounts.yeTeamRefresh.trigger'), value: flow.value?.trigger === 'automatic_401' ? t('admin.accounts.yeTeamRefresh.automatic401') : flow.value?.trigger === 'manual' ? t('admin.accounts.yeTeamRefresh.manual') : formatValue(flow.value?.trigger) },
  { label: t('admin.accounts.yeTeamRefresh.startedAt'), value: formatStageTime(flow.value?.started_at) },
  { label: t('admin.accounts.yeTeamRefresh.finishedAt'), value: formatStageTime(flow.value?.finished_at) },
  { label: t('admin.accounts.yeTeamRefresh.duration'), value: duration.value },
  { label: t('admin.accounts.yeTeamRefresh.fallbackUsed'), value: flow.value?.fallback_used ? t('admin.accounts.yeTeamRefresh.fallbackYes') : t('admin.accounts.yeTeamRefresh.fallbackNo') },
  { label: t('admin.accounts.yeTeamRefresh.orderNo'), value: formatValue(flow.value?.order_no) },
  { label: t('admin.accounts.yeTeamRefresh.packageCount'), value: formatValue(flow.value?.package_count) },
  { label: t('admin.accounts.yeTeamRefresh.credentialChanged'), value: flow.value?.credential_changed === undefined ? '-' : flow.value.credential_changed ? t('admin.accounts.yeTeamRefresh.yes') : t('admin.accounts.yeTeamRefresh.no') },
  { label: t('admin.accounts.yeTeamRefresh.cacheInvalidated'), value: flow.value?.cache_invalidated ? t('admin.accounts.yeTeamRefresh.yes') : t('admin.accounts.yeTeamRefresh.no') }
])
const batchItems = computed(() => {
  const batch = flow.value?.batch
  if (!batch) return []
  return ['total', 'queued', 'already_running', 'done', 'failed', 'unreclaimable', 'not_owned', 'skipped', 'cards', 'tasks'].map(key => ({
    key,
    label: t(`admin.accounts.yeTeamRefresh.batchFields.${key}`),
    value: formatValue(batch[key as keyof typeof batch])
  }))
})
const taskItems = (task: NonNullable<YeTeamRefreshFlow['task']>) => {
  return [
    { label: t('admin.accounts.yeTeamRefresh.currentStatus'), value: formatValue(task.status), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.orderNo'), value: formatValue(task.order_no), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.resourceUid'), value: formatValue(task.resource_uid), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.providerStatus'), value: formatValue(task.provider_status), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.errorCode'), value: formatValue(task.error_code), tone: task.error_code ? 'text-red-700 dark:text-red-300' : '' },
    { label: t('admin.accounts.yeTeamRefresh.failureClass'), value: formatValue(task.failure_class), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.permanent'), value: task.permanent ? t('admin.accounts.yeTeamRefresh.permanentYes') : t('admin.accounts.yeTeamRefresh.permanentNo'), tone: '' },
    { label: t('admin.accounts.yeTeamRefresh.message'), value: formatValue(task.message), tone: task.message ? 'text-red-700 dark:text-red-300' : '' }
  ]
}
const stageLabel = (name: string) => {
  const key = `admin.accounts.yeTeamRefresh.stages.${name}`
  const translated = t(key)
  return translated === key ? name : translated
}
const stageStatusLabel = (status: YeTeamRefreshStage['status']) => t(`admin.accounts.yeTeamRefresh.stageStatus.${status}`)
const stageDotClass = (status: YeTeamRefreshStage['status']) => status === 'success' ? 'bg-emerald-500' : status === 'failed' ? 'bg-red-500' : 'bg-blue-500 animate-pulse'
const stageTextClass = (status: YeTeamRefreshStage['status']) => status === 'success' ? 'font-medium text-emerald-600 dark:text-emerald-400' : status === 'failed' ? 'font-medium text-red-600 dark:text-red-400' : 'font-medium text-blue-600 dark:text-blue-400'
</script>
