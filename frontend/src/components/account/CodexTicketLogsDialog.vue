<template>
  <BaseDialog :show="show" :title="`${account.name} #${account.id}`" width="full" @close="emit('close')">
    <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-2">
        <div class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ model }}</div>
        <div v-if="snapshot?.status" class="flex items-center gap-3 text-xs">
          <span :class="codexTicketStateClass(snapshot.status.state)">{{ t(`admin.accounts.codexTickets.states.${snapshot.status.state}`) }}</span>
          <span class="tabular-nums text-gray-500 dark:text-gray-400">{{ t('admin.accounts.codexTickets.attempts', { count: snapshot.status.attempts }) }}</span>
        </div>
      </div>
      <span class="inline-flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400">
        <span class="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true"></span>{{ t('admin.accounts.codexTickets.autoRefresh') }}
      </span>
    </div>
    <div v-if="failed" role="alert" class="mb-3 flex items-center justify-between gap-3 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
      {{ t('admin.accounts.codexTickets.loadFailed') }}
      <button type="button" class="shrink-0 underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :disabled="loading" @click="refresh">{{ t('admin.accounts.codexTickets.retry') }}</button>
    </div>
    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600" :aria-busy="loading && !snapshot">
      <table class="w-full min-w-[1000px] table-fixed text-left text-xs">
        <caption class="sr-only">{{ t('admin.accounts.codexTickets.viewLogs', { model }) }}</caption>
        <thead class="border-b border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400">
          <tr>
            <th v-for="column in columns" :key="column.key" scope="col" class="px-3 py-2.5 font-normal" :style="{ width: column.width }">{{ t(`admin.accounts.codexTickets.columns.${column.key}`) }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="entry in entries" :key="entry.id" class="text-gray-700 dark:text-gray-200">
            <td class="whitespace-nowrap px-3 py-3 tabular-nums">{{ formatTime(entry.time) }}</td>
            <td class="px-3 py-3 tabular-nums">{{ entry.attempt }}</td>
            <td class="px-3 py-3" :class="eventClass(entry.event)">{{ t(`admin.accounts.codexTickets.events.${entry.event}`) }}</td>
            <td class="px-3 py-3 leading-5">{{ reason(entry.reason) }}</td>
            <td class="px-3 py-3 tabular-nums">{{ entry.http_status ?? '—' }}</td>
            <td class="break-all px-3 py-3 font-mono text-[11px]">
              <span v-if="entry.egress_ip">{{ entry.egress_ip }}</span>
              <span v-else-if="entry.egress_error" tabindex="0" class="cursor-help border-b border-dotted border-red-400 text-red-500" :title="egressReason(entry)">{{ t('admin.accounts.codexTickets.egressFailed') }}</span>
              <span v-else class="text-gray-400">—</span>
            </td>
            <td class="px-3 py-3">{{ countryName(entry.egress_country_code) }}</td>
            <td class="whitespace-nowrap px-3 py-3 tabular-nums">{{ entry.ticket_length ?? '—' }} / {{ entry.target_length }}</td>
            <td class="whitespace-nowrap px-3 py-3 tabular-nums">{{ entry.duration_ms == null ? '—' : `${entry.duration_ms} ms` }}</td>
          </tr>
          <tr v-if="!entries.length"><td :colspan="columns.length" class="py-12 text-center text-gray-400">{{ t(loading ? 'admin.accounts.codexTickets.loading' : 'admin.accounts.codexTickets.empty') }}</td></tr>
        </tbody>
      </table>
    </div>
    <div class="mt-3 space-y-1 text-[11px] leading-5 text-gray-500 dark:text-gray-400">
      <div>{{ t('admin.accounts.codexTickets.lastUpdated') }} {{ snapshot ? formatTime(snapshot.fetched_at) : '—' }}</div>
      <div>{{ t('admin.accounts.codexTickets.retention', { count: snapshot?.limit ?? 200 }) }}</div>
      <div>{{ t('admin.accounts.codexTickets.egressHint') }}</div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getCodexTicketLogs } from '@/api/admin/codexTickets'
import type { Account } from '@/types'
import type { CodexTicketLogEntry, CodexTicketLogsResponse, CodexTicketStatus } from '@/types/codexTicket'
import { codexTicketStateClass } from '@/utils/codexTicketDisplay'

const props = defineProps<{ show: boolean; account: Account; model: string }>()
const emit = defineEmits<{ close: []; status: [status: CodexTicketStatus] }>()
const { t, locale } = useI18n()
const snapshot = ref<CodexTicketLogsResponse | null>(null)
const loading = ref(false)
const failed = ref(false)
const entries = computed(() => [...(snapshot.value?.entries ?? [])].sort((a, b) => b.id - a.id))
const columns = [
  { key: 'time', width: '14%' }, { key: 'attempt', width: '8%' }, { key: 'event', width: '9%' },
  { key: 'reason', width: '22%' }, { key: 'http', width: '6%' }, { key: 'ip', width: '12%' },
  { key: 'country', width: '10%' }, { key: 'length', width: '11%' }, { key: 'duration', width: '8%' }
]
const reasonKeys = new Set(['request_started', 'target_length_matched', 'http_error', 'missing_state', 'invalid_prefix', 'length_mismatch', 'timeout', 'canceled', 'network_error', 'empty_response', 'request_error'])
const reason = (value: string) => t(`admin.accounts.codexTickets.reasons.${reasonKeys.has(value) ? value : 'request_error'}`)
function eventClass(event: string) {
  return codexTicketStateClass(event === 'acquired' ? 'ready' : event === 'started' ? 'harvesting' : event === 'error' ? 'token_invalid' : 'cooldown')
}
function egressReason(entry: CodexTicketLogEntry) {
  return `${t('admin.accounts.codexTickets.egressDiagnostic')}: ${entry.egress_error?.reason ?? 'unknown'}${entry.egress_error?.http_status ? ` (HTTP ${entry.egress_error.http_status})` : ''}`
}
function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')} ${date.toLocaleTimeString(locale?.value ?? 'zh-CN', { hour12: false })}`
}
function countryName(code?: string) {
  if (!code || !/^[A-Z]{2}$/.test(code)) return '—'
  // Keep compatibility with ES2020 TS libs while using modern browser region names.
  const DisplayNames = (Intl as unknown as { DisplayNames?: new (locales: string[], options: { type: string }) => { of(code: string): string | undefined } }).DisplayNames
  try { return DisplayNames ? new DisplayNames([locale?.value ?? 'zh-CN'], { type: 'region' }).of(code) ?? code : code } catch { return code }
}
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let generation = 0
let disposed = false
const isVisible = () => document.visibilityState !== 'hidden'
function stop() {
  generation++
  controller?.abort(); controller = undefined
  if (timer) clearTimeout(timer)
  timer = undefined
  loading.value = false
}
async function refresh() {
  if (disposed || !props.show || loading.value || !isVisible()) return
  if (timer) clearTimeout(timer)
  const run = generation
  const pending = new AbortController()
  controller = pending; loading.value = true
  try {
    const result = await getCodexTicketLogs(props.account.id, props.model, pending.signal)
    if (disposed || run !== generation || pending.signal.aborted) return
    snapshot.value = result; failed.value = false
    if (result.status) emit('status', result.status)
  } catch {
    if (run === generation && !pending.signal.aborted) failed.value = true
  } finally {
    if (run === generation) {
      loading.value = false; controller = undefined
      if (!disposed && props.show && isVisible()) timer = setTimeout(refresh, 2000)
    }
  }
}
function visibilityChanged() { if (!isVisible()) stop(); else void refresh() }
watch(() => [props.show, props.account.id, props.model], () => {
  stop(); snapshot.value = null; failed.value = false
  if (props.show) void refresh()
}, { immediate: true })
document.addEventListener('visibilitychange', visibilityChanged)
onBeforeUnmount(() => { disposed = true; stop(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>
