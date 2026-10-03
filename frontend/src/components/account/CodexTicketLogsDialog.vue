<template>
  <BaseDialog :show="show" :title="`${account.name} #${account.id}`" width="full" @close="emit('close')">
    <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-2">
        <div class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ model }}</div>
        <div v-if="snapshot?.status" class="flex items-center gap-3 text-xs">
          <span :class="codexTicketStateClass(codexTicketState(snapshot.status))">{{ t(`admin.accounts.codexTickets.states.${codexTicketState(snapshot.status)}`) }}</span>
          <span class="tabular-nums text-gray-500 dark:text-gray-400">{{ t('admin.accounts.codexTickets.attempts', { count: codexTicketAttempts(snapshot.status) }) }}</span>
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
    <div v-if="blacklistFailed" role="alert" class="mb-3 rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ t('admin.accounts.codexTickets.blacklistFailed') }}</div>
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
            <td class="px-3 py-3">
              <div class="flex flex-col items-start gap-1.5">
                <span class="break-all font-mono text-[11px]">{{ entry.gateway || '—' }}</span>
                <button
                  v-if="canBlacklist(entry.gateway)" type="button"
                  class="rounded px-1.5 py-1 text-[11px] text-primary-600 ring-1 ring-primary-200 hover:bg-primary-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-default disabled:text-gray-400 disabled:ring-gray-200 dark:text-primary-400 dark:ring-primary-800 dark:hover:bg-primary-900/20 dark:disabled:ring-dark-600"
                  :disabled="!!blacklistSaving || isBlacklisted(entry.gateway)"
                  :aria-label="t('admin.accounts.codexTickets.addGatewayToBlacklist', { gateway: entry.gateway })"
                  @click="addToBlacklist(entry.gateway!)"
                >{{ t(isBlacklisted(entry.gateway) ? 'admin.accounts.codexTickets.gatewayBlacklisted' : blacklistSaving === entry.gateway ? 'admin.accounts.codexTickets.blacklistSaving' : 'admin.accounts.codexTickets.addToBlacklist') }}</button>
              </div>
            </td>
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
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { addCodexTicketGatewayToBlacklist, getCodexTicketLogs } from '@/api/admin/codexTickets'
import type { Account } from '@/types'
import type { CodexTicketLogsResponse, CodexTicketStatus } from '@/types/codexTicket'
import { codexTicketAttempts, codexTicketState, codexTicketStateClass } from '@/utils/codexTicketDisplay'
import { normalizeCodexGatewayName, readCodexGatewayBlacklist } from '@/utils/codexGatewayBlacklist'

const props = defineProps<{ show: boolean; account: Account; model: string }>()
const emit = defineEmits<{ close: []; status: [status: CodexTicketStatus]; 'account-updated': [account: Account] }>()
const { t, locale } = useI18n()
const snapshot = ref<CodexTicketLogsResponse | null>(null)
const loading = ref(false)
const failed = ref(false)
const blacklistSaving = ref('')
const blacklistFailed = ref(false)
const gatewayBlacklist = ref<string[]>([])
const canBlacklist = (gateway?: string) => !!gateway && /^unified-\d{1,5}$/.test(normalizeCodexGatewayName(gateway))
const isBlacklisted = (gateway?: string) => !!gateway && gatewayBlacklist.value.includes(normalizeCodexGatewayName(gateway))
const entries = computed(() => [...(snapshot.value?.entries ?? [])].sort((a, b) => b.id - a.id))
const columns = [
  { key: 'time', width: '14%' }, { key: 'attempt', width: '8%' }, { key: 'event', width: '9%' },
  { key: 'reason', width: '25%' }, { key: 'http', width: '6%' }, { key: 'gateway', width: '16%' },
  { key: 'length', width: '14%' }, { key: 'duration', width: '8%' }
]
const reasonKeys = new Set(['request_started', 'target_length_matched', 'http_error', 'missing_state', 'invalid_prefix', 'length_mismatch', 'timeout', 'canceled', 'network_error', 'empty_response', 'request_error'])
const reason = (value: string) => t(`admin.accounts.codexTickets.reasons.${reasonKeys.has(value) ? value : 'request_error'}`)
function eventClass(event: string) {
  return codexTicketStateClass(event === 'acquired' ? 'ready' : event === 'started' ? 'harvesting' : event === 'error' ? 'token_invalid' : 'cooldown')
}
function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')} ${date.toLocaleTimeString(locale?.value ?? 'zh-CN', { hour12: false })}`
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
    if (result.gateway_blacklist && !blacklistSaving.value) gatewayBlacklist.value = result.gateway_blacklist
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
let mutationGeneration = 0
async function addToBlacklist(gateway: string) {
  if (!canBlacklist(gateway) || isBlacklisted(gateway) || blacklistSaving.value) return
  const run = mutationGeneration
  blacklistSaving.value = gateway
  blacklistFailed.value = false
  try {
    const account = await addCodexTicketGatewayToBlacklist(props.account.id, normalizeCodexGatewayName(gateway))
    if (disposed || run !== mutationGeneration) return
    stop()
    gatewayBlacklist.value = readCodexGatewayBlacklist(account.extra)
    blacklistSaving.value = ''
    emit('account-updated', account)
    void refresh()
  } catch {
    if (!disposed && run === mutationGeneration) blacklistFailed.value = true
  } finally {
    if (run === mutationGeneration) blacklistSaving.value = ''
  }
}
function visibilityChanged() { if (!isVisible()) stop(); else void refresh() }
watch(() => [props.show, props.account.id, props.model], () => {
  stop(); snapshot.value = null; failed.value = false
  mutationGeneration++
  blacklistSaving.value = ''; blacklistFailed.value = false
  gatewayBlacklist.value = readCodexGatewayBlacklist(props.account.extra)
  if (props.show) void refresh()
}, { immediate: true })
document.addEventListener('visibilitychange', visibilityChanged)
onBeforeUnmount(() => { disposed = true; stop(); document.removeEventListener('visibilitychange', visibilityChanged) })
</script>
