<template>
  <AppLayout>
    <section class="md3-dashboard">
      <header class="md3-dashboard-header">
        <div class="min-w-0">
          <p class="md3-dashboard-kicker">{{ t('nav.dashboard') }}</p>
          <h1>{{ t('dashboard.title') }}</h1>
          <p>{{ t('dashboard.welcomeMessage') }}</p>
        </div>
        <div class="md3-dashboard-actions">
          <button
            type="button"
            class="md3-tonal-button"
            :disabled="dashboardBusy"
            :title="t('common.refresh')"
            @click="refreshAll"
          >
            <Icon name="refresh" size="sm" />
            <span>{{ t('common.refresh') }}</span>
          </button>
          <button
            v-if="dailyCheckinStatus?.enabled"
            type="button"
            class="md3-filled-button"
            :disabled="dailyCheckinDisabled"
            :title="dailyCheckinTitle"
            @click="handleDailyCheckin"
          >
            <Icon :name="dailyCheckinStatus.checked_in_today ? 'checkCircle' : 'gift'" size="sm" />
            <span>{{ dailyCheckinButtonText }}</span>
          </button>
        </div>
      </header>

      <div v-if="loading" class="md3-dashboard-loading">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <UserDashboardStats
          :stats="stats"
          :balance="user?.balance || 0"
          :is-simple="authStore.isSimpleMode"
          :platform-quotas="platformQuotas"
        />
        <UserDashboardCharts
          v-model:startDate="startDate"
          v-model:endDate="endDate"
          v-model:granularity="granularity"
          :loading="loadingCharts"
          :trend="trendData"
          :models="modelStats"
          @dateRangeChange="loadCharts"
          @granularityChange="loadCharts"
          @refresh="refreshAll"
        />
        <div class="md3-dashboard-main-grid">
          <UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" />
          <UserDashboardQuickActions />
        </div>
      </template>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import Icon from '@/components/icons/Icon.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem, DailyCheckinStatus } from '@/types'
import { getMyPlatformQuotas, getDailyCheckinStatus, claimDailyCheckin } from '@/api/user'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const user = computed(() => authStore.user)

const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
const dailyCheckinStatus = ref<DailyCheckinStatus | null>(null)
const dailyCheckinLoading = ref(false)

const formatLD = (d: Date) => d.toISOString().split('T')[0]
type DashboardGranularity = 'day' | 'hour'
const startDate = ref(formatLD(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLD(new Date()))
const granularity = ref<DashboardGranularity>('day')

const loadStats = async () => {
  loading.value = true
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  try {
    const res = await Promise.all([
      usageAPI.getDashboardTrend({
        start_date: startDate.value,
        end_date: endDate.value,
        granularity: granularity.value
      }),
      usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })
    ])
    trendData.value = res[0].trend || []
    modelStats.value = res[1].models || []
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadRecent = async () => {
  loadingUsage.value = true
  try {
    const res = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = res.items.slice(0, 5)
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

const loadPlatformQuotas = async () => {
  try {
    const data = await getMyPlatformQuotas()
    platformQuotas.value = data.platform_quotas ?? []
  } catch (error) {
    console.warn('Failed to load platform quotas:', error)
    platformQuotas.value = []
  }
}

const loadDailyCheckin = async () => {
  try {
    dailyCheckinStatus.value = await getDailyCheckinStatus()
  } catch (error) {
    console.warn('Failed to load daily check-in status:', error)
    dailyCheckinStatus.value = null
  }
}

const dailyCheckinDisabled = computed(() => dailyCheckinLoading.value || !dailyCheckinStatus.value?.enabled || dailyCheckinStatus.value.checked_in_today || dailyCheckinStatus.value.exhausted_today)
const dailyCheckinButtonText = computed(() => {
  if (dailyCheckinLoading.value) return t('dashboard.dailyCheckin.checking')
  if (dailyCheckinStatus.value?.checked_in_today) return t('dashboard.dailyCheckin.checked')
  if (dailyCheckinStatus.value?.exhausted_today) return t('dashboard.dailyCheckin.exhausted')
  return t('dashboard.dailyCheckin.action')
})
const dailyCheckinTitle = computed(() => {
  const status = dailyCheckinStatus.value
  if (!status) return ''
  if (status.checked_in_today) return t('dashboard.dailyCheckin.checkedHint', { amount: formatCurrency(status.today_reward) })
  if (status.exhausted_today) return t('dashboard.dailyCheckin.exhaustedHint')
  return t('dashboard.dailyCheckin.hint', { min: formatCurrency(status.min_reward), max: formatCurrency(status.max_reward) })
})
const formatCurrency = (value: number) => `$${Number(value || 0).toFixed(2)}`
const dashboardBusy = computed(() => loading.value || loadingUsage.value || loadingCharts.value || dailyCheckinLoading.value)
const refreshAll = () => {
  loadStats()
  loadCharts()
  loadRecent()
  loadPlatformQuotas()
  loadDailyCheckin()
}
const handleDailyCheckin = async () => {
  if (dailyCheckinDisabled.value) return
  dailyCheckinLoading.value = true
  try {
    const result = await claimDailyCheckin()
    dailyCheckinStatus.value = result
    appStore.showSuccess(t('dashboard.dailyCheckin.success', { amount: formatCurrency(result.reward) }))
    await authStore.refreshUser()
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'dashboard.dailyCheckin.errors', t('dashboard.dailyCheckin.failed')))
    await loadDailyCheckin()
  } finally {
    dailyCheckinLoading.value = false
  }
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.md3-dashboard {
  display: grid;
  gap: 20px;
}

.md3-dashboard-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 16px 8px 4px;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.dark .md3-dashboard-header {
  background: transparent;
  box-shadow: none;
}

.md3-dashboard-kicker {
  margin: 0 0 6px;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 700;
  text-transform: uppercase;
}

.dark .md3-dashboard-kicker {
  color: var(--md-primary);
}

.md3-dashboard-header h1 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 2rem;
  line-height: 1.2;
  font-weight: 800;
}

.dark .md3-dashboard-header h1 {
  color: var(--md-on-surface);
}

.md3-dashboard-header p:not(.md3-dashboard-kicker) {
  margin: 8px 0 0;
  max-width: 42rem;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  line-height: 1.6;
}

.dark .md3-dashboard-header p:not(.md3-dashboard-kicker) {
  color: var(--md-on-surface-variant);
}

.md3-dashboard-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.md3-tonal-button,
.md3-filled-button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 999px;
  padding: 0 16px;
  font-size: 0.875rem;
  font-weight: 700;
  transition: background-color 160ms ease, border-color 160ms ease, color 160ms ease;
}

.md3-tonal-button {
  border: 1px solid transparent;
  background: var(--md-primary-container);
  color: var(--md-on-primary-container);
}

.md3-tonal-button:hover:not(:disabled) {
  background: var(--md-surface-container-high);
}

.md3-filled-button {
  border: 1px solid transparent;
  background: var(--md-brand);
  color: var(--md-on-brand);
}

.md3-filled-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-brand) 88%, black);
}

.md3-tonal-button:disabled,
.md3-filled-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.dark .md3-tonal-button {
  background: var(--md-primary-container);
  color: var(--md-on-primary-container);
}

.dark .md3-tonal-button:hover:not(:disabled) {
  background: var(--md-surface-container-high);
}

.dark .md3-filled-button {
  background: var(--md-brand);
  color: var(--md-on-brand);
}

.dark .md3-filled-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-brand) 88%, black);
}

.md3-dashboard-loading {
  display: flex;
  min-height: 280px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.dark .md3-dashboard-loading {
  border-color: var(--md-outline-variant);
  background: var(--md-surface);
}

.md3-dashboard-main-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(280px, 0.85fr);
  gap: 20px;
  align-items: start;
}

@media (max-width: 1024px) {
  .md3-dashboard-main-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 768px) {
  .md3-dashboard-header {
    flex-direction: column;
  }

  .md3-dashboard-actions {
    width: 100%;
    justify-content: stretch;
  }

  .md3-tonal-button,
  .md3-filled-button {
    flex: 1 1 160px;
  }
}
</style>
