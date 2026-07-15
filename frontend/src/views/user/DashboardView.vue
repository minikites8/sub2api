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
            data-testid="daily-checkin-entry"
            type="button"
            class="btn btn-primary inline-flex min-w-[8rem] items-center justify-center gap-2"
            :disabled="dailyCheckinLoading"
            :title="dailyCheckinTitle"
            @click="openDailyCheckinDialog"
          >
            <Icon :name="dailyCheckinEntryIcon" size="sm" :stroke-width="2" />
            <span>{{ dailyCheckinEntryText }}</span>
          </button>
        </div>
      </header>

      <BaseDialog
        :show="showDailyCheckinDialog"
        :title="t('dashboard.dailyCheckin.title')"
        width="narrow"
        :close-on-click-outside="true"
        @close="closeDailyCheckinDialog"
      >
        <template #title>
          <span class="inline-flex items-center gap-3">
            <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300">
              <Icon name="gift" size="md" :stroke-width="2" />
            </span>
            <span>{{ t('dashboard.dailyCheckin.title') }}</span>
          </span>
        </template>
        <div v-if="dailyCheckinStatus">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <template v-if="dailyCheckinStatus.checked_in_today">
              <div class="flex items-start gap-3">
                <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300">
                  <Icon name="checkCircle" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">
                    {{ t('dashboard.dailyCheckin.checked') }}
                  </p>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                    {{ t('dashboard.dailyCheckin.checkedHint', { amount: formatCurrency(dailyCheckinStatus.today_reward) }) }}
                  </p>
                </div>
              </div>
            </template>

            <template v-else-if="dailyCheckinStatus.exhausted_today">
              <div class="flex items-start gap-3">
                <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-300">
                  <Icon name="exclamationCircle" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">
                    {{ t('dashboard.dailyCheckin.exhausted') }}
                  </p>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                    {{ t('dashboard.dailyCheckin.exhaustedHint') }}
                  </p>
                </div>
              </div>
            </template>

            <template v-else-if="!dailyCheckinRechargeEligible">
              <div class="space-y-4">
                <div class="flex items-start gap-3">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
                    <Icon name="creditCard" size="md" :stroke-width="2" />
                  </div>
                  <div>
                    <p class="text-sm font-semibold text-gray-900 dark:text-white">
                      {{ t('dashboard.dailyCheckin.rechargeRequired') }}
                    </p>
                    <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                      {{ t('dashboard.dailyCheckin.rechargeRequiredHint') }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  class="btn btn-primary inline-flex w-full items-center justify-center gap-2"
                  @click="goRecharge"
                >
                  <Icon name="creditCard" size="sm" :stroke-width="2" />
                  <span>{{ t('dashboard.dailyCheckin.goRecharge') }}</span>
                </button>
              </div>
            </template>

            <template v-else>
              <div class="space-y-3">
                <div class="flex items-start gap-3">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon name="shield" size="md" :stroke-width="2" />
                  </div>
                  <div>
                    <p class="text-sm font-semibold text-gray-900 dark:text-white">
                      {{ t('dashboard.dailyCheckin.verifyTitle') }}
                    </p>
                    <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                      {{ t('dashboard.dailyCheckin.verifyHint') }}
                    </p>
                  </div>
                </div>

                  <GoogleAdSenseAd
                    v-if="dailyCheckinStatus.ads_enabled"
                    client="ca-pub-1423021104870807"
                    ad-slot="5962250608"
                  />

                <div v-if="publicSettingsLoading" class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-dark-400">
                  {{ t('dashboard.dailyCheckin.loadingVerification') }}
                </div>
                <TurnstileWidget
                  v-else-if="turnstileReady"
                  ref="turnstileRef"
                  :site-key="turnstileSiteKey"
                  @verify="onTurnstileVerify"
                  @expire="onTurnstileExpire"
                  @error="onTurnstileError"
                />
                <div v-else class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-3 text-sm text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                  {{ t('dashboard.dailyCheckin.verificationUnavailable') }}
                </div>

                <p v-if="turnstileError" class="text-sm text-rose-600 dark:text-rose-300">
                  {{ turnstileError }}
                </p>

                <button
                  type="button"
                  class="btn btn-primary inline-flex w-full items-center justify-center gap-2"
                  :disabled="dailyCheckinDisabled"
                  @click="handleDailyCheckin"
                >
                  <Icon :name="dailyCheckinButtonIcon" size="sm" :stroke-width="2" />
                  <span>{{ dailyCheckinButtonText }}</span>
                </button>
              </div>
            </template>
          </div>
        </div>
      </BaseDialog>

      <div v-if="loading" class="md3-dashboard-loading">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
      </template>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import GoogleAdSenseAd from '@/components/ads/GoogleAdSenseAd.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrendDataPoint, ModelStat, DailyCheckinStatus } from '@/types'
import { getDailyCheckinStatus, claimDailyCheckin } from '@/api/user'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { isDailyCheckinRechargeEligible } from '@/utils/dailyCheckin'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const user = computed(() => authStore.user)

const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingCharts = ref(false)
const publicSettingsLoading = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const dailyCheckinStatus = ref<DailyCheckinStatus | null>(null)
const dailyCheckinLoading = ref(false)
const showDailyCheckinDialog = ref(false)
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref('')
const turnstileError = ref('')

const formatLD = (d: Date) => d.toISOString().split('T')[0]
const startDate = ref(formatLD(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLD(new Date()))
const granularity = ref('day')

const formatCurrency = (value: number) => `$${Number(value || 0).toFixed(2)}`

const turnstileSiteKey = computed(() => appStore.cachedPublicSettings?.turnstile_site_key || '')
const turnstileReady = computed(() => Boolean(appStore.cachedPublicSettings?.turnstile_enabled && turnstileSiteKey.value))
const dailyCheckinAvailable = computed(() => {
  const status = dailyCheckinStatus.value
  return Boolean(status?.enabled && dailyCheckinRechargeEligible.value && !status.checked_in_today && !status.exhausted_today)
})
const dailyCheckinRechargeEligible = computed(() => {
  const status = dailyCheckinStatus.value
  return status ? isDailyCheckinRechargeEligible(status) : false
})
const dailyCheckinDisabled = computed(() => {
  return dailyCheckinLoading.value || publicSettingsLoading.value || !dailyCheckinAvailable.value || !turnstileReady.value || !turnstileToken.value
})
const dashboardBusy = computed(() => loading.value || loadingCharts.value || dailyCheckinLoading.value)
const dailyCheckinTitle = computed(() => {
  const status = dailyCheckinStatus.value
  if (!status) return ''
  if (status.checked_in_today) return t('dashboard.dailyCheckin.checkedHint', { amount: formatCurrency(status.today_reward) })
  if (status.exhausted_today) return t('dashboard.dailyCheckin.exhaustedHint')
  if (!dailyCheckinRechargeEligible.value) return t('dashboard.dailyCheckin.rechargeRequiredHint')
  return t('dashboard.dailyCheckin.hint')
})
const dailyCheckinEntryIcon = computed(() => {
  const status = dailyCheckinStatus.value
  if (dailyCheckinLoading.value) return 'refresh'
  if (status?.checked_in_today) return 'checkCircle'
  if (status?.exhausted_today) return 'exclamationCircle'
  return 'gift'
})
const dailyCheckinEntryText = computed(() => {
  const status = dailyCheckinStatus.value
  if (dailyCheckinLoading.value) return t('dashboard.dailyCheckin.checking')
  if (status?.checked_in_today) return t('dashboard.dailyCheckin.checked')
  if (status?.exhausted_today) return t('dashboard.dailyCheckin.exhausted')
  return t('dashboard.dailyCheckin.action')
})
const dailyCheckinButtonIcon = computed(() => {
  if (dailyCheckinLoading.value) return 'refresh'
  if (!turnstileToken.value) return 'shield'
  return 'gift'
})
const dailyCheckinButtonText = computed(() => {
  if (dailyCheckinLoading.value) return t('dashboard.dailyCheckin.checking')
  if (publicSettingsLoading.value) return t('dashboard.dailyCheckin.loadingVerification')
  if (!turnstileReady.value) return t('dashboard.dailyCheckin.verificationRequired')
  if (!turnstileToken.value) return t('dashboard.dailyCheckin.completeVerification')
  return t('dashboard.dailyCheckin.action')
})

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
      usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }),
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

const loadDailyCheckin = async () => {
  try {
    dailyCheckinStatus.value = await getDailyCheckinStatus()
  } catch (error) {
    console.warn('Failed to load daily check-in status:', error)
    dailyCheckinStatus.value = null
  }
}

const loadPublicSettings = async () => {
  publicSettingsLoading.value = true
  try {
    await appStore.fetchPublicSettings()
  } finally {
    publicSettingsLoading.value = false
  }
}

const refreshAll = () => {
  loadStats()
  loadCharts()
  loadDailyCheckin()
  loadPublicSettings()
}

const resetTurnstile = () => {
  turnstileRef.value?.reset()
  turnstileToken.value = ''
}

const openDailyCheckinDialog = () => {
  showDailyCheckinDialog.value = true
}

const closeDailyCheckinDialog = () => {
  showDailyCheckinDialog.value = false
  resetTurnstile()
  turnstileError.value = ''
}

const onTurnstileVerify = (token: string) => {
  turnstileToken.value = token
  turnstileError.value = ''
}

const onTurnstileExpire = () => {
  turnstileToken.value = ''
  turnstileError.value = t('dashboard.dailyCheckin.turnstileExpired')
}

const onTurnstileError = () => {
  turnstileToken.value = ''
  turnstileError.value = t('dashboard.dailyCheckin.turnstileFailed')
}

const goRecharge = () => {
  closeDailyCheckinDialog()
  router.push('/purchase')
}

const handleDailyCheckin = async () => {
  if (!dailyCheckinAvailable.value) {
    if (dailyCheckinStatus.value && !dailyCheckinRechargeEligible.value) {
      goRecharge()
    }
    return
  }
  if (!turnstileReady.value) {
    appStore.showError(t('dashboard.dailyCheckin.verificationUnavailable'))
    return
  }
  if (!turnstileToken.value) {
    turnstileError.value = t('dashboard.dailyCheckin.completeVerification')
    appStore.showWarning(turnstileError.value)
    return
  }

  dailyCheckinLoading.value = true
  try {
    const result = await claimDailyCheckin({ turnstile_token: turnstileToken.value })
    dailyCheckinStatus.value = result
    appStore.showSuccess(t('dashboard.dailyCheckin.success', { amount: formatCurrency(result.reward) }))
    resetTurnstile()
    await authStore.refreshUser()
  } catch (error) {
    resetTurnstile()
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
  gap: 16px;
}

.md3-dashboard-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 2px;
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
  margin: 0 0 4px;
  color: var(--md-on-surface-variant);
  font-size: 0.6875rem;
  font-weight: 650;
  text-transform: uppercase;
}

.dark .md3-dashboard-kicker {
  color: var(--md-on-surface-variant);
}

.md3-dashboard-header h1 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 1.5rem;
  line-height: 1.2;
  font-weight: 700;
}

.dark .md3-dashboard-header h1 {
  color: var(--md-on-surface);
}

.md3-dashboard-header p:not(.md3-dashboard-kicker) {
  margin: 6px 0 0;
  max-width: 42rem;
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
  line-height: 1.5;
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
