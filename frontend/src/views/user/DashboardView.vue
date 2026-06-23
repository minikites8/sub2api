<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <div v-if="dailyCheckinStatus?.enabled" class="flex justify-end">
          <button
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
                        {{ t('dashboard.dailyCheckin.rechargeRequiredHint', { amount: formatCurrency(dailyCheckinStatus.min_recharge_amount), current: formatCurrency(dailyCheckinStatus.total_recharged) }) }}
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

        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
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
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import GoogleAdSenseAd from '@/components/ads/GoogleAdSenseAd.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem, DailyCheckinStatus } from '@/types'
import { getMyPlatformQuotas, getDailyCheckinStatus, claimDailyCheckin } from '@/api/user'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { isDailyCheckinRechargeEligible } from '@/utils/dailyCheckin'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const user = computed(() => authStore.user)

const stats = ref<UserStatsType | null>(null)
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const publicSettingsLoading = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)
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
const dailyCheckinTitle = computed(() => {
  const status = dailyCheckinStatus.value
  if (!status) return ''
  if (status.checked_in_today) return t('dashboard.dailyCheckin.checkedHint', { amount: formatCurrency(status.today_reward) })
  if (status.exhausted_today) return t('dashboard.dailyCheckin.exhaustedHint')
  if (!dailyCheckinRechargeEligible.value) return t('dashboard.dailyCheckin.rechargeRequiredHint', { amount: formatCurrency(status.min_recharge_amount), current: formatCurrency(status.total_recharged) })
  return t('dashboard.dailyCheckin.hint', { min: formatCurrency(status.min_reward), max: formatCurrency(status.max_reward) })
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
  loadRecent()
  loadPlatformQuotas()
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
