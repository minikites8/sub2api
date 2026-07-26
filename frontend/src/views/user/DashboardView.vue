<template>
  <AppLayout>
    <section class="telemetry-dashboard">
      <header class="telemetry-dashboard-header">
        <div class="min-w-0">
          <p class="telemetry-dashboard-kicker">{{ t('nav.dashboard') }}</p>
          <h1>{{ t('dashboard.overview') }}</h1>
          <p>{{ t('dashboard.telemetry') }}</p>
        </div>
        <div class="telemetry-dashboard-actions">
          <button
            type="button"
            class="telemetry-secondary-button"
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
            class="telemetry-primary-button"
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

      <div v-if="loading" class="telemetry-dashboard-loading">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <UserDashboardStats
          :stats="stats"
          :balance="dashboardBalance"
          :is-simple="dashboardSimpleMode"
        />
        <UserDashboardCharts
          v-model:startDate="startDate"
          v-model:endDate="endDate"
          v-model:granularity="granularity"
          :loading="loadingCharts"
          :trend="trendData"
          :models="modelStats"
          :api-keys="activeApiKeys"
          :api-keys-loading="loadingApiKeys"
          @dateRangeChange="loadCharts"
          @granularityChange="loadCharts"
          @refresh="refreshAll"
        />
      </template>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { keysAPI } from '@/api/keys'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import GoogleAdSenseAd from '@/components/ads/GoogleAdSenseAd.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrendDataPoint, ModelStat, DailyCheckinStatus, ApiKey } from '@/types'
import { getDailyCheckinStatus, claimDailyCheckin } from '@/api/user'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatCredits, usdToCredits } from '@/utils/credit'
import { isDailyCheckinRechargeEligible } from '@/utils/dailyCheckin'
import type { DashboardPreviewData } from '@/mocks/dashboardPreview'

const props = withDefaults(defineProps<{
  previewData?: DashboardPreviewData
}>(), {
  previewData: undefined
})

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const user = computed(() => authStore.user)
const previewMode = import.meta.env.DEV && Boolean(props.previewData)

const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingCharts = ref(false)
const publicSettingsLoading = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const activeApiKeys = ref<ApiKey[]>([])
const loadingApiKeys = ref(false)
const dailyCheckinStatus = ref<DailyCheckinStatus | null>(null)
const dailyCheckinLoading = ref(false)
const showDailyCheckinDialog = ref(false)
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref('')
const turnstileError = ref('')

const dashboardBalance = computed(() => previewMode ? props.previewData?.balance || 0 : user.value?.balance || 0)
const dashboardSimpleMode = computed(() => previewMode ? false : authStore.isSimpleMode)

const formatLD = (d: Date) => d.toISOString().split('T')[0]
const startDate = ref(formatLD(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLD(new Date()))
const granularity = ref('day')

// Check-in rewards are credited to the USD balance; the UI presents them in Credits.
const formatCurrency = (value: number) => `${formatCredits(usdToCredits(value))} Credits`

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
const dashboardBusy = computed(
  () => loading.value || loadingCharts.value || loadingApiKeys.value || dailyCheckinLoading.value
)
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
    if (previewMode) {
      stats.value = props.previewData?.stats || null
    } else {
      await authStore.refreshUser()
      stats.value = await usageAPI.getDashboardStats()
    }
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  try {
    if (previewMode) {
      trendData.value = props.previewData?.trend || []
      modelStats.value = props.previewData?.models || []
    } else {
      const res = await Promise.all([
        usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }),
        usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })
      ])
      trendData.value = res[0].trend || []
      modelStats.value = res[1].models || []
    }
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadApiKeys = async () => {
  loadingApiKeys.value = true
  try {
    if (previewMode) {
      activeApiKeys.value = props.previewData?.apiKeys || []
    } else {
      const response = await keysAPI.list(1, 3, {
        status: 'active',
        sort_by: 'updated_at',
        sort_order: 'desc'
      })
      activeApiKeys.value = response.items
    }
  } catch (error) {
    console.warn('Failed to load dashboard API keys:', error)
    activeApiKeys.value = []
  } finally {
    loadingApiKeys.value = false
  }
}

const loadDailyCheckin = async () => {
  try {
    dailyCheckinStatus.value = previewMode
      ? props.previewData?.dailyCheckinStatus || null
      : await getDailyCheckinStatus()
  } catch (error) {
    console.warn('Failed to load daily check-in status:', error)
    dailyCheckinStatus.value = null
  }
}

const loadPublicSettings = async () => {
  if (previewMode) return
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
  loadApiKeys()
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

const htmlElement = document.documentElement
const hadDarkTheme = htmlElement.classList.contains('dark')

onMounted(() => {
  htmlElement.classList.add('dark')
  refreshAll()
})

onBeforeUnmount(() => {
  htmlElement.classList.toggle('dark', hadDarkTheme)
})
</script>

<style scoped>
.telemetry-dashboard {
  --md-surface: #061016;
  --md-surface-container-low: #101b22;
  --md-surface-container: #152129;
  --md-surface-container-high: #1d2a31;
  --md-on-surface: #e6f0eb;
  --md-on-surface-variant: #8da198;
  --md-outline-variant: #303b3a;
  --md-primary: #00e38b;
  --md-chart-1: #00e38b;
  --md-chart-2: #508eff;
  --md-chart-3: #b3c7ff;
  --md-chart-4: #56ffa8;
  --md-chart-5: #e1bd68;
  --md-chart-6: #ff8d84;
  --md-chart-7: #b9cbbc;
  --md-chart-8: #849587;
  display: grid;
  min-height: calc(100vh - 64px);
  gap: 32px;
  margin: -24px -32px;
  background-color: #0b141c;
  background-image:
    linear-gradient(rgb(59 74 63 / 13%) 1px, transparent 1px),
    linear-gradient(90deg, rgb(59 74 63 / 13%) 1px, transparent 1px);
  background-size: 32px 32px;
  padding: 42px 48px 80px;
  color: #dae3ee;
}

.telemetry-dashboard-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
}

.telemetry-dashboard-kicker {
  margin-bottom: 9px;
  color: #00e38b;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.67rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.telemetry-dashboard-header h1 {
  color: #f4fff8;
  font-size: clamp(2.1rem, 4vw, 3rem);
  font-weight: 760;
  line-height: 1.08;
  letter-spacing: 0;
}

.telemetry-dashboard-header p:not(.telemetry-dashboard-kicker) {
  margin-top: 10px;
  color: #91a69c;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.78rem;
  line-height: 1.55;
}

.telemetry-dashboard-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.telemetry-secondary-button,
.telemetry-primary-button {
  display: inline-flex;
  min-height: 42px;
  align-items: center;
  justify-content: center;
  gap: 9px;
  border-radius: 4px;
  padding: 0 16px;
  font-size: 0.78rem;
  font-weight: 750;
  transition: border-color 160ms ease, background-color 160ms ease, color 160ms ease;
}

.telemetry-secondary-button {
  border: 1px solid #31443c;
  background: #101b22;
  color: #c5d2cc;
}

.telemetry-secondary-button:hover:enabled {
  border-color: #00e38b;
  color: #00e38b;
}

.telemetry-primary-button {
  min-width: 128px;
  border: 1px solid #00e38b;
  background: #00e38b;
  color: #03130d;
}

.telemetry-primary-button:hover:enabled {
  background: #17f09b;
}

.telemetry-secondary-button:disabled,
.telemetry-primary-button:disabled {
  cursor: wait;
  opacity: 0.55;
}

.telemetry-dashboard-loading {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
  border: 1px solid #303b3a;
  border-radius: 6px;
  background: #061016;
}

@media (max-width: 1024px) {
  .telemetry-dashboard {
    padding-inline: 32px;
  }
}

@media (max-width: 768px) {
  .telemetry-dashboard {
    gap: 24px;
    margin: -20px -16px;
    padding: 30px 20px 64px;
  }

  .telemetry-dashboard-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .telemetry-dashboard-actions {
    width: 100%;
    justify-content: stretch;
  }

  .telemetry-secondary-button,
  .telemetry-primary-button {
    flex: 1 1 148px;
  }
}
</style>
