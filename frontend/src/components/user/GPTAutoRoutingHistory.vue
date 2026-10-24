<template>
  <section v-if="status && status.has_history" class="card" data-testid="gpt-auto-history">
    <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAutoHistory.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAutoHistory.description', { model: status.virtual_model }) }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Select v-model="range" data-testid="gpt-auto-history-range" @change="reload" :options="(rangeOptions).map((option) => ({ value: option, label: t(`gptAutoHistory.ranges.${option}`) }))" />
        <template v-if="status.export_enabled">
          <Select v-model="format" data-testid="gpt-auto-history-format" :options="[{ value: 'csv', label: 'CSV' }, { value: 'jsonl', label: 'JSONL' }]" />
          <button type="button" class="btn btn-secondary" :disabled="exporting" data-testid="gpt-auto-history-export" @click="exportLogs">
            {{ exporting ? t('gptAutoHistory.exporting') : t('gptAutoHistory.export') }}
          </button>
        </template>
      </div>
    </header>

    <p v-if="notice" class="mx-6 mt-4 rounded-lg p-3 text-xs" :class="noticeWarning ? 'bg-amber-50 text-amber-800 dark:bg-amber-900/20 dark:text-amber-200' : 'bg-gray-50 text-gray-700 dark:bg-dark-800 dark:text-gray-300'" data-testid="gpt-auto-history-notice">{{ notice }}</p>
    <p v-if="error" role="alert" class="mx-6 mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>

    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th v-for="column in columns" :key="column" class="whitespace-nowrap px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(`gptAutoHistory.columns.${column}`) }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-if="loading"><td :colspan="columns.length" class="px-3 py-6 text-center text-gray-500">{{ t('common.loading') }}</td></tr>
          <tr v-else-if="items.length === 0"><td :colspan="columns.length" class="px-3 py-6 text-center text-gray-500" data-testid="gpt-auto-history-empty">{{ t('gptAutoHistory.empty') }}</td></tr>
          <tr v-for="item in items" v-else :key="item.request_id + item.created_at">
            <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">{{ formatDateTime(item.created_at) }}</td>
            <td class="max-w-[10rem] truncate px-3 py-2 font-mono text-xs" :title="item.request_id">{{ item.request_id || '—' }}</td>
            <td class="px-3 py-2 font-mono text-xs">{{ item.executed_model || '—' }}</td>
            <td class="px-3 py-2">{{ item.reasoning_effort || '—' }}</td>
            <td class="px-3 py-2">{{ item.status ? t(`gptAutoHistory.statuses.${item.status}`) : '—' }}</td>
            <td class="px-3 py-2 tabular-nums">{{ item.total_latency_ms !== undefined ? `${item.total_latency_ms} ms` : '—' }}</td>
            <td class="px-3 py-2 text-xs text-gray-600 dark:text-gray-400">
              <span v-if="item.fallback" class="mr-1 rounded-full bg-amber-50 px-2 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('gptAutoHistory.fallback') }}</span>
              {{ item.routing_explanation }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <Pagination v-if="total > pageSize" :page="page" :total="total" :page-size="pageSize" @update:page="onPage" />
    <p class="px-6 pb-4 pt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAutoHistory.chargesNote') }}</p>
  </section>
</template>

<script setup lang="ts">
import Select from '@/components/common/Select.vue'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Pagination from '@/components/common/Pagination.vue'
import { gptAutoUserAPI, type GPTAutoCustomerExportFormat, type GPTAutoCustomerLogEntry, type GPTAutoCustomerStatus } from '@/api/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { formatBytes, saveBlob } from '@/utils/gptAuto'

const { t } = useI18n()

type RangeKey = '1d' | '7d' | '30d' | '90d'
const rangeOptions: RangeKey[] = ['1d', '7d', '30d', '90d']
const rangeDays: Record<RangeKey, number> = { '1d': 1, '7d': 7, '30d': 30, '90d': 90 }
const columns = ['time', 'requestId', 'model', 'effort', 'status', 'latency', 'routing']

const status = ref<GPTAutoCustomerStatus | null>(null)
const range = ref<RangeKey>('7d')
const format = ref<GPTAutoCustomerExportFormat>('csv')
const items = ref<GPTAutoCustomerLogEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const exporting = ref(false)
const error = ref('')
const notice = ref('')
const noticeWarning = ref(false)

function window() {
  const end = new Date()
  return { start_time: new Date(end.getTime() - rangeDays[range.value] * 86400000).toISOString(), end_time: end.toISOString() }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await gptAutoUserAPI.listRoutingLogs({ ...window(), page: page.value, page_size: pageSize })
    items.value = result.items ?? []
    total.value = result.total
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAutoHistory.loadFailed'))
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  void load()
}

function onPage(next: number) {
  page.value = next
  void load()
}

async function exportLogs() {
  exporting.value = true
  error.value = ''
  notice.value = ''
  try {
    const params = window()
    // The count validates the request and reports truncation before the
    // download; a streamed file cannot carry a readable error.
    const summary = await gptAutoUserAPI.countRoutingLogExport(params)
    if (summary.rows === 0) {
      notice.value = t('gptAutoHistory.nothingToExport')
      noticeWarning.value = false
      return
    }
    const file = await gptAutoUserAPI.exportRoutingLogs({ ...params, format: format.value })
    saveBlob(file.blob, file.filename)
    noticeWarning.value = summary.truncated
    notice.value = summary.truncated
      ? t('gptAutoHistory.truncated', { rows: summary.rows, max: summary.max_rows })
      : t('gptAutoHistory.exported', { rows: summary.rows, size: formatBytes(file.blob.size) })
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAutoHistory.exportFailed'))
  } finally {
    exporting.value = false
  }
}

onMounted(async () => {
  try {
    status.value = await gptAutoUserAPI.getStatus()
  } catch {
    // The section stays hidden when the feature endpoint is unavailable.
    status.value = null
    return
  }
  if (status.value.has_history) await load()
})
</script>
