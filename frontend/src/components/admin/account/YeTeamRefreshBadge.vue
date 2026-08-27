<template>
  <div
    v-if="hasBinding"
    data-testid="ye-team-refresh-state"
    class="mt-1 flex min-w-0 max-w-[280px] items-center gap-1.5"
    :title="title"
  >
    <span
      data-testid="ye-team-refresh-badge"
      :class="[
        'inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium leading-4',
        badgeClass
      ]"
    >
      <span :class="['h-1.5 w-1.5 rounded-full', dotClass]" />
      {{ label }}
    </span>
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTimeToMinute } from '@/utils/format'
import type { Account } from '@/types'

type RefreshState = 'unrefreshed' | 'success' | 'failed'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()

const extra = computed<Record<string, unknown>>(() => props.account.extra || {})
const hasBinding = computed(() => {
  const cardCode = extra.value.ye_team_card_code
  return typeof cardCode === 'string' && cardCode.trim().length > 0
})
const state = computed<RefreshState>(() => {
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
const label = computed(() => t(`admin.accounts.yeTeamRefresh.${state.value === 'success' ? 'refreshed' : state.value === 'failed' ? 'failed' : 'notRefreshed'}`))
const badgeClass = computed(() => {
  if (state.value === 'success') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (state.value === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})
const dotClass = computed(() => {
  if (state.value === 'success') return 'bg-emerald-500'
  if (state.value === 'failed') return 'bg-red-500'
  return 'bg-gray-400'
})
const title = computed(() => {
  const parts = [label.value]
  if (formattedAt.value) parts.push(formattedAt.value)
  if (state.value === 'failed' && lastError.value) parts.push(lastError.value)
  return parts.join(' · ')
})
</script>
