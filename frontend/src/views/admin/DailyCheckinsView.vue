<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="openSettingsDialog">
            <Icon name="cog" size="sm" />
            <span>{{ t('admin.dailyCheckins.settings.button') }}</span>
          </button>
        </div>
      </template>

      <template #filters>
        <div class="space-y-4">
          <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.dailyCheckins.progress.title') }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  {{ t('admin.dailyCheckins.progress.date', { date: settingsSummary?.checkin_date || '-' }) }}
                </p>
              </div>
              <div class="text-left sm:text-right">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.dailyCheckins.progress.used', { used: formatReward(progressUsed), limit: formatReward(progressLimit) }) }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  {{ t('admin.dailyCheckins.progress.remaining', { amount: formatReward(progressRemaining) }) }}
                </p>
              </div>
            </div>
            <progress
              class="mt-4 h-2 w-full overflow-hidden rounded-full [&::-moz-progress-bar]:rounded-full [&::-moz-progress-bar]:bg-primary-600 [&::-webkit-progress-bar]:rounded-full [&::-webkit-progress-bar]:bg-gray-100 [&::-webkit-progress-value]:rounded-full [&::-webkit-progress-value]:bg-primary-600 dark:[&::-webkit-progress-bar]:bg-dark-700"
              max="100"
              :value="dailyProgressPercent"
              :aria-label="t('admin.dailyCheckins.progress.title')"
            />
          </div>

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
            <button class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('common.refresh')" @click="refreshPage">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
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

    <BaseDialog
      :show="settingsDialogOpen"
      :title="t('admin.dailyCheckins.settings.title')"
      width="normal"
      @close="closeSettingsDialog"
    >
      <div v-if="settingsLoading" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('common.loading') }}
      </div>
      <form v-else id="daily-checkin-settings-form" class="space-y-5" @submit.prevent="saveSettings">
        <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60">
          <span>
            <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dailyCheckins.settings.enabled') }}</span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.dailyCheckins.settings.enabledHint') }}</span>
          </span>
          <input
            v-model="settingsForm.enabled"
            type="checkbox"
            class="mt-0.5 h-5 w-5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </label>

        <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60">
          <span>
            <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dailyCheckins.settings.adsEnabled') }}</span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.dailyCheckins.settings.adsEnabledHint') }}</span>
          </span>
          <input
            v-model="settingsForm.ads_enabled"
            type="checkbox"
            class="mt-0.5 h-5 w-5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
        </label>

        <div class="grid gap-4 sm:grid-cols-2">
          <label class="space-y-1.5">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.minReward') }}</span>
            <input
              v-model.number="settingsForm.min_reward"
              type="number"
              min="0"
              step="0.00000001"
              class="input"
            />
          </label>
          <label class="space-y-1.5">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.maxReward') }}</span>
            <input
              v-model.number="settingsForm.max_reward"
              type="number"
              min="0"
              step="0.00000001"
              class="input"
            />
          </label>
        </div>

        <label class="block space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.dailyTotalLimit') }}</span>
          <input
            v-model.number="settingsForm.daily_total_limit"
            type="number"
            min="0"
            step="0.00000001"
            class="input"
          />
        </label>

        <label class="block space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.minRechargeAmount') }}</span>
          <input
            v-model.number="settingsForm.min_recharge_amount"
            type="number"
            min="0"
            step="0.00000001"
            class="input"
          />
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.dailyCheckins.settings.minRechargeAmountHint') }}</span>
        </label>

        <div class="space-y-3 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dailyCheckins.settings.rewardTiers') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.dailyCheckins.settings.rewardTiersHint') }}</p>
            </div>
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.dailyCheckins.settings.weightTotal', { amount: formatPercent(rewardTierWeightTotal) }) }}
            </p>
          </div>

          <div class="space-y-3">
            <div
              v-for="(tier, index) in rewardTierForm"
              :key="index"
              class="grid gap-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900/40 sm:grid-cols-[minmax(0,1fr)_8rem_8rem]"
            >
              <div class="min-w-0">
                <p class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.dailyCheckins.settings.tierLabel', { index: index + 1 }) }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                  {{ t('admin.dailyCheckins.settings.tierRange', { start: formatPercent(tierLowerPercent(index)), end: formatPercent(tier.upper_ratio_percent) }) }}
                </p>
              </div>
              <label class="space-y-1">
                <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.upperRatio') }}</span>
                <input
                  v-model.number="tier.upper_ratio_percent"
                  type="number"
                  min="0"
                  max="100"
                  step="0.000001"
                  class="input"
                  :disabled="index === rewardTierForm.length - 1"
                />
              </label>
              <label class="space-y-1">
                <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.dailyCheckins.settings.weight') }}</span>
                <input
                  v-model.number="tier.weight_percent"
                  type="number"
                  min="0"
                  step="0.000001"
                  class="input"
                />
              </label>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="closeSettingsDialog">{{ t('common.cancel') }}</button>
          <button
            type="submit"
            form="daily-checkin-settings-form"
            class="btn btn-primary"
            :disabled="settingsSaving || settingsLoading"
          >
            {{ settingsSaving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import dailyCheckinsAPI, { type DailyCheckinRecord, type DailyCheckinRewardTier, type DailyCheckinSettings, type ListDailyCheckinRecordsParams } from '@/api/admin/dailyCheckins'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDisplayDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const records = ref<DailyCheckinRecord[]>([])
const filters = reactive({ search: '', start_date: '', end_date: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const settingsDialogOpen = ref(false)
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const settingsSummary = ref<DailyCheckinSettings | null>(null)
interface DailyCheckinRewardTierForm {
  upper_ratio_percent: number
  weight_percent: number
}

const settingsForm = reactive<DailyCheckinSettings>({
  enabled: false,
  ads_enabled: true,
  daily_total_limit: 0,
  min_reward: 0,
  max_reward: 0,
  min_recharge_amount: 0,
  reward_tiers: [],
  today_total_granted: 0,
  remaining_today: 0,
  exhausted_today: false,
  checkin_date: '',
})
const rewardTierForm = ref<DailyCheckinRewardTierForm[]>([])
let debounceTimer: ReturnType<typeof setTimeout> | null = null

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.dailyCheckins.columns.user'), sortable: true },
  { key: 'checkin_date', label: t('admin.dailyCheckins.columns.checkinDate'), sortable: true },
  { key: 'reward', label: t('admin.dailyCheckins.columns.reward'), sortable: true },
  { key: 'created_at', label: t('admin.dailyCheckins.columns.createdAt'), sortable: true },
])

const sortState = reactive(loadInitialSortState())

const progressUsed = computed(() => Number(settingsSummary.value?.today_total_granted) || 0)
const progressLimit = computed(() => Number(settingsSummary.value?.daily_total_limit) || 0)
const progressRemaining = computed(() => Number(settingsSummary.value?.remaining_today) || 0)
const dailyProgressPercent = computed(() => {
  if (progressLimit.value <= 0) return 0
  return Math.min(100, Math.max(0, (progressUsed.value / progressLimit.value) * 100))
})
const rewardTierWeightTotal = computed(() => {
  return rewardTierForm.value.reduce((sum, tier) => sum + (Number(tier.weight_percent) || 0), 0)
})

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

function refreshPage() {
  void Promise.all([loadRecords(), loadSettingsSummary()])
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

async function openSettingsDialog() {
  settingsDialogOpen.value = true
  await loadSettings()
}

function closeSettingsDialog() {
  if (settingsSaving.value) return
  settingsDialogOpen.value = false
}

async function loadSettings() {
  settingsLoading.value = true
  try {
    assignSettings(await dailyCheckinsAPI.getSettings())
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.dailyCheckins.errors', t('admin.dailyCheckins.errors.settingsLoadFailed')))
  } finally {
    settingsLoading.value = false
  }
}

async function loadSettingsSummary() {
  try {
    assignSettings(await dailyCheckinsAPI.getSettings())
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.dailyCheckins.errors', t('admin.dailyCheckins.errors.settingsLoadFailed')))
  }
}

async function saveSettings() {
  settingsSaving.value = true
  try {
    assignSettings(await dailyCheckinsAPI.updateSettings({
      enabled: settingsForm.enabled,
      ads_enabled: settingsForm.ads_enabled,
      daily_total_limit: Number(settingsForm.daily_total_limit) || 0,
      min_reward: Number(settingsForm.min_reward) || 0,
      max_reward: Number(settingsForm.max_reward) || 0,
      min_recharge_amount: Number(settingsForm.min_recharge_amount) || 0,
      reward_tiers: buildRewardTiersPayload(),
    }))
    appStore.showSuccess(t('admin.dailyCheckins.settings.saved'))
    settingsDialogOpen.value = false
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'admin.dailyCheckins.errors', t('admin.dailyCheckins.errors.settingsSaveFailed')))
  } finally {
    settingsSaving.value = false
  }
}

function assignSettings(settings: DailyCheckinSettings) {
  settingsSummary.value = { ...settings }
  settingsForm.enabled = !!settings.enabled
  settingsForm.ads_enabled = settings.ads_enabled !== false
  settingsForm.daily_total_limit = Number(settings.daily_total_limit) || 0
  settingsForm.min_reward = Number(settings.min_reward) || 0
  settingsForm.max_reward = Number(settings.max_reward) || 0
  settingsForm.min_recharge_amount = Number(settings.min_recharge_amount) || 0
  settingsForm.reward_tiers = normalizeRewardTiers(settings.reward_tiers)
  settingsForm.today_total_granted = Number(settings.today_total_granted) || 0
  settingsForm.remaining_today = Number(settings.remaining_today) || 0
  settingsForm.exhausted_today = !!settings.exhausted_today
  settingsForm.checkin_date = settings.checkin_date || ''
  rewardTierForm.value = settingsForm.reward_tiers.map((tier) => ({
    upper_ratio_percent: ratioToPercent(tier.upper_ratio),
    weight_percent: ratioToPercent(tier.weight),
  }))
}

function formatReward(value: number | null | undefined): string {
  const rounded = Number(value || 0).toFixed(8).replace(/0+$/, '').replace(/\.$/, '')
  return `$${rounded || '0'}`
}

function formatDateTime(value: string | null | undefined): string {
  return value ? formatDisplayDateTime(value) : '-'
}

const defaultRewardTiers: DailyCheckinRewardTier[] = [
  { upper_ratio: 0.1, weight: 0.5 },
  { upper_ratio: 0.35, weight: 0.3 },
  { upper_ratio: 0.75, weight: 0.15 },
  { upper_ratio: 1, weight: 0.05 },
]

function normalizeRewardTiers(tiers: DailyCheckinRewardTier[] | null | undefined): DailyCheckinRewardTier[] {
  const source = tiers && tiers.length > 0 ? tiers : defaultRewardTiers
  return source.map((tier, index) => ({
    upper_ratio: index === source.length - 1 ? 1 : Number(tier.upper_ratio) || 0,
    weight: Number(tier.weight) || 0,
  }))
}

function ratioToPercent(value: number): number {
  return Math.round((Number(value) || 0) * 10000000000) / 100000000
}

function percentToRatio(value: number): number {
  return Math.round((Number(value) || 0) * 1000000) / 100000000
}

function formatPercent(value: number): string {
  const rounded = Number(value || 0).toFixed(6).replace(/0+$/, '').replace(/\.$/, '')
  return `${rounded || '0'}%`
}

function tierLowerPercent(index: number): number {
  if (index <= 0) return 0
  return Number(rewardTierForm.value[index - 1]?.upper_ratio_percent) || 0
}

function buildRewardTiersPayload(): DailyCheckinRewardTier[] {
  return rewardTierForm.value.map((tier, index) => ({
    upper_ratio: index === rewardTierForm.value.length - 1 ? 1 : percentToRatio(tier.upper_ratio_percent),
    weight: percentToRatio(tier.weight_percent),
  }))
}

function recordKey(row: DailyCheckinRecord): string {
  return `${row.user_id}:${row.checkin_date}`
}

onMounted(() => {
  void loadRecords()
  void loadSettingsSummary()
})
</script>
