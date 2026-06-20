<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full md:w-80">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="filters.search"
              type="text"
              class="input pl-10"
              :placeholder="t('admin.dailyCheckins.searchPlaceholder')"
              @input="debounceLoad"
            />
          </div>
          <input
            v-model="filters.start_date"
            type="date"
            class="input w-full sm:w-44"
            :title="t('admin.dailyCheckins.startDate')"
            @change="reloadFromFirstPage"
          />
          <input
            v-model="filters.end_date"
            type="date"
            class="input w-full sm:w-44"
            :title="t('admin.dailyCheckins.endDate')"
            @change="reloadFromFirstPage"
          />
          <button class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('common.refresh')" @click="loadRecords">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="records"
          :loading="loading"
          :server-side-sort="true"
          :row-key="recordKey"
          default-sort-key="created_at"
          default-sort-order="desc"
          sort-storage-key="admin-daily-checkins-table-sort"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="space-y-0.5">
              <div class="font-mono text-sm text-gray-900 dark:text-white">#{{ row.user_id }}</div>
              <div class="max-w-56 truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.email || '-' }}</div>
              <div class="max-w-56 truncate text-sm text-gray-500 dark:text-dark-400">{{ row.username || '-' }}</div>
            </div>
          </template>
          <template #cell-reward="{ row }">
            <span class="text-sm font-semibold text-emerald-600 dark:text-emerald-400">{{ formatReward(row.reward) }}</span>
          </template>
          <template #cell-checkin_date="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">{{ row.checkin_date || '-' }}</span>
          </template>
          <template #cell-created_at="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</span>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import dailyCheckinsAPI, { type DailyCheckinRecord, type ListDailyCheckinRecordsParams } from '@/api/admin/dailyCheckins'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDisplayDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const records = ref<DailyCheckinRecord[]>([])
const filters = reactive({ search: '', start_date: '', end_date: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
let debounceTimer: ReturnType<typeof setTimeout> | null = null

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.dailyCheckins.columns.user'), sortable: true },
  { key: 'checkin_date', label: t('admin.dailyCheckins.columns.checkinDate'), sortable: true },
  { key: 'reward', label: t('admin.dailyCheckins.columns.reward'), sortable: true },
  { key: 'created_at', label: t('admin.dailyCheckins.columns.createdAt'), sortable: true },
])

const sortState = reactive(loadInitialSortState())

function loadInitialSortState(): { sort_by: string; sort_order: 'asc' | 'desc' } {
  const fallback = { sort_by: 'created_at', sort_order: 'desc' as const }
  try {
    const raw = localStorage.getItem('admin-daily-checkins-table-sort')
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as { key?: string; order?: string }
    const key = typeof parsed.key === 'string' ? parsed.key : ''
    if (!columns.value.some((column) => column.key === key && column.sortable)) return fallback
    return {
      sort_by: key,
      sort_order: parsed.order === 'asc' ? 'asc' : 'desc',
    }
  } catch {
    return fallback
  }
}

function buildParams(): ListDailyCheckinRecordsParams {
  return {
    page: pagination.page,
    page_size: pagination.page_size,
    search: filters.search.trim() || undefined,
    start_date: filters.start_date || undefined,
    end_date: filters.end_date || undefined,
    sort_by: sortState.sort_by,
    sort_order: sortState.sort_order,
  }
}

async function loadRecords() {
  loading.value = true
  try {
    const res = await dailyCheckinsAPI.listRecords(buildParams())
    records.value = res.items || []
    pagination.total = res.total || 0
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.dailyCheckins.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function debounceLoad() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => reloadFromFirstPage(), 300)
}

function reloadFromFirstPage() {
  pagination.page = 1
  void loadRecords()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadRecords()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void loadRecords()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  void loadRecords()
}

function formatReward(value: number | null | undefined): string {
  const rounded = Number(value || 0).toFixed(8).replace(/0+$/, '').replace(/\.$/, '')
  return `$${rounded || '0'}`
}

function formatDateTime(value: string | null | undefined): string {
  return value ? formatDisplayDateTime(value) : '-'
}

function recordKey(row: DailyCheckinRecord): string {
  return `${row.user_id}:${row.checkin_date}`
}

onMounted(() => {
  void loadRecords()
})
</script>
