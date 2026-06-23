<template>
  <AppLayout>
    <div class="redeem-page mx-auto space-y-6">
      <section class="redeem-workspace">
        <header class="redeem-header">
          <div>
            <h1>{{ t('redeem.title') }}</h1>
            <p>{{ t('redeem.accountLine', { account: accountDisplayName }) }}</p>
          </div>
          <button
            type="button"
            class="redeem-icon-button"
            :disabled="loadingHistory"
            :title="t('common.refresh')"
            :aria-label="t('common.refresh')"
            @click="refreshRedeemData"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingHistory }" />
          </button>
        </header>

        <div class="redeem-balance-card">
          <div>
            <span>{{ t('redeem.currentBalance') }}</span>
            <div class="redeem-balance-value">
              <span>$</span>
              <strong>{{ currentBalanceText }}</strong>
            </div>
          </div>
          <div class="redeem-balance-meta">
            <Icon name="bolt" size="sm" />
            <span>{{ t('redeem.concurrency') }}: {{ currentConcurrency }} {{ t('redeem.requests') }}</span>
          </div>
        </div>
      </section>

      <section class="redeem-grid">
        <article class="redeem-panel redeem-code-panel">
          <header class="redeem-panel-header">
            <h2>{{ t('redeem.redeemCodeLabel') }}</h2>
          </header>

          <div class="redeem-panel-body">
            <form class="redeem-form" @submit.prevent="handleRedeem">
              <label class="redeem-code-field" for="code">
                <span>{{ t('redeem.redeemCodeLabel') }}</span>
                <input
                  id="code"
                  v-model="redeemCode"
                  type="text"
                  required
                  :placeholder="t('redeem.redeemCodePlaceholder')"
                  :disabled="submitting"
                  autocomplete="off"
                />
              </label>
              <p class="redeem-field-hint">{{ t('redeem.redeemCodeHint') }}</p>

              <button
                type="submit"
                class="redeem-submit-button"
                :disabled="!redeemCode || submitting"
              >
                <span v-if="submitting" class="flex items-center justify-center gap-2">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
                  {{ t('redeem.redeeming') }}
                </span>
                <span v-else>{{ t('redeem.redeemButton') }}</span>
              </button>
            </form>

            <div class="redeem-account-note">
              {{ t('redeem.accountLine', { account: accountDisplayName }) }}
            </div>

            <transition name="fade">
              <div v-if="redeemResult" class="redeem-result-card redeem-result-success">
                <div class="redeem-result-icon">
                  <Icon name="checkCircle" size="sm" />
                </div>
                <div>
                  <h3>{{ t('redeem.redeemSuccess') }}</h3>
                  <p>{{ redeemResult.message }}</p>
                  <div class="redeem-result-details">
                    <p v-if="redeemResult.type === 'balance'">
                      {{ t('redeem.added') }}: ${{ redeemResult.value.toFixed(2) }}
                    </p>
                    <p v-else-if="redeemResult.type === 'concurrency'">
                      {{ t('redeem.added') }}: {{ redeemResult.value }}
                      {{ t('redeem.concurrentRequests') }}
                    </p>
                    <p v-else-if="redeemResult.type === 'subscription'">
                      {{ t('redeem.subscriptionAssigned') }}
                      <span v-if="redeemResult.group_name"> - {{ redeemResult.group_name }}</span>
                      <span v-if="redeemResult.validity_days">
                        ({{ t('redeem.subscriptionDays', { days: redeemResult.validity_days }) }})
                      </span>
                    </p>
                    <p v-if="redeemResult.new_balance !== undefined">
                      {{ t('redeem.newBalance') }}:
                      <strong>${{ redeemResult.new_balance.toFixed(2) }}</strong>
                    </p>
                    <p v-if="redeemResult.new_concurrency !== undefined">
                      {{ t('redeem.newConcurrency') }}:
                      <strong>{{ redeemResult.new_concurrency }} {{ t('redeem.requests') }}</strong>
                    </p>
                  </div>
                </div>
              </div>
            </transition>

            <transition name="fade">
              <div v-if="errorMessage" class="redeem-result-card redeem-result-error">
                <div class="redeem-result-icon">
                  <Icon name="exclamationCircle" size="sm" />
                </div>
                <div>
                  <h3>{{ t('redeem.redeemFailed') }}</h3>
                  <p>{{ errorMessage }}</p>
                </div>
              </div>
            </transition>
          </div>
        </article>

        <article class="redeem-panel redeem-rules-panel">
          <header class="redeem-panel-header">
            <h2>{{ t('redeem.aboutCodes') }}</h2>
          </header>

          <div class="redeem-panel-body">
            <div class="redeem-rules-intro">
              <Icon name="infoCircle" size="sm" />
              <p>{{ t('redeem.description') }}</p>
            </div>

            <ul class="redeem-rule-list">
              <li>
                <span>01</span>
                <p>{{ t('redeem.codeRule1') }}</p>
              </li>
              <li>
                <span>02</span>
                <p>{{ t('redeem.codeRule2') }}</p>
              </li>
              <li>
                <span>03</span>
                <p>{{ t('redeem.codeRule3') }}</p>
              </li>
              <li>
                <span>04</span>
                <p>{{ t('redeem.codeRule4') }}</p>
              </li>
            </ul>

            <div v-if="contactInfo" class="redeem-contact-card">
              <span>{{ t('redeem.contact') }}</span>
              <strong>{{ contactInfo }}</strong>
            </div>
          </div>
        </article>
      </section>

      <section class="redeem-history-section">
        <div class="redeem-history-rule"></div>
        <header class="redeem-history-header">
          <div>
            <h2>{{ t('redeem.recentActivity') }}</h2>
            <p>{{ t('redeem.historyWillAppear') }}</p>
          </div>
          <div class="redeem-history-actions">
            <button
              type="button"
              :disabled="loadingHistory"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              @click="fetchHistory"
            >
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingHistory }" />
            </button>
          </div>
        </header>

        <div class="redeem-history-table">
          <div v-if="loadingHistory" class="redeem-history-empty">
            {{ t('common.processing') }}
          </div>

          <div v-else-if="history.length > 0" class="redeem-history-list">
            <div v-for="item in history" :key="item.id" class="redeem-history-row">
              <div class="redeem-history-main">
                <div
                  class="redeem-history-marker"
                  :class="`redeem-history-marker-${getHistoryTone(item)}`"
                >
                  <Icon :name="getHistoryIcon(item)" size="sm" />
                </div>
                <div class="min-w-0">
                  <p>{{ getHistoryItemTitle(item) }}</p>
                  <span>{{ formatDateTime(item.used_at) }}</span>
                </div>
              </div>
              <div class="redeem-history-value">
                <strong :class="`redeem-history-value-${getHistoryTone(item)}`">
                  {{ formatHistoryValue(item) }}
                </strong>
                <span v-if="!isAdminAdjustment(item.type)" class="redeem-history-code">
                  {{ item.code.slice(0, 8) }}...
                </span>
                <span v-else>{{ t('redeem.adminAdjustment') }}</span>
                <span v-if="item.notes" class="redeem-history-notes" :title="item.notes">
                  {{ item.notes }}
                </span>
              </div>
            </div>
          </div>

          <div v-else class="redeem-history-empty">
            {{ t('redeem.historyWillAppear') }}
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { redeemAPI, authAPI, type RedeemHistoryItem } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)
const accountDisplayName = computed(() => user.value?.email || user.value?.username || '-')
const currentBalanceText = computed(() => {
  const balance = Number(user.value?.balance ?? 0)
  return Number.isFinite(balance) ? balance.toFixed(2) : '0.00'
})
const currentConcurrency = computed(() => user.value?.concurrency || 0)

const redeemCode = ref('')
const submitting = ref(false)
const redeemResult = ref<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
  group_name?: string
  validity_days?: number
} | null>(null)
const errorMessage = ref('')

// History data
const history = ref<RedeemHistoryItem[]>([])
const loadingHistory = ref(false)
const contactInfo = ref('')

// Helper functions for history display
const isBalanceType = (type: string) => {
  return type === 'balance' || type === 'admin_balance'
}

const isSubscriptionType = (type: string) => {
  return type === 'subscription'
}

const isAdminAdjustment = (type: string) => {
  return type === 'admin_balance' || type === 'admin_concurrency'
}

type HistoryIcon = 'dollar' | 'badge' | 'bolt'
type HistoryTone = 'positive' | 'negative' | 'subscription' | 'concurrency'

const getHistoryTone = (item: RedeemHistoryItem): HistoryTone => {
  if (item.value < 0) {
    return 'negative'
  }
  if (isSubscriptionType(item.type)) {
    return 'subscription'
  }
  if (isBalanceType(item.type)) {
    return 'positive'
  }
  return 'concurrency'
}

const getHistoryIcon = (item: RedeemHistoryItem): HistoryIcon => {
  if (isBalanceType(item.type)) {
    return 'dollar'
  }
  if (isSubscriptionType(item.type)) {
    return 'badge'
  }
  return 'bolt'
}

const getHistoryItemTitle = (item: RedeemHistoryItem) => {
  if (item.type === 'balance') {
    return t('redeem.balanceAddedRedeem')
  } else if (item.type === 'admin_balance') {
    return item.value >= 0 ? t('redeem.balanceAddedAdmin') : t('redeem.balanceDeductedAdmin')
  } else if (item.type === 'concurrency') {
    return t('redeem.concurrencyAddedRedeem')
  } else if (item.type === 'admin_concurrency') {
    return item.value >= 0 ? t('redeem.concurrencyAddedAdmin') : t('redeem.concurrencyReducedAdmin')
  } else if (item.type === 'subscription') {
    return t('redeem.subscriptionAssigned')
  }
  return t('common.unknown')
}

const formatHistoryValue = (item: RedeemHistoryItem) => {
  if (isBalanceType(item.type)) {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}$${item.value.toFixed(2)}`
  } else if (isSubscriptionType(item.type)) {
    // 订阅类型显示有效天数和分组名称
    const days = item.validity_days || Math.round(item.value)
    const groupName = item.group?.name || ''
    return groupName ? `${days}${t('redeem.days')} - ${groupName}` : `${days}${t('redeem.days')}`
  } else {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}${item.value} ${t('redeem.requests')}`
  }
}

const fetchHistory = async () => {
  loadingHistory.value = true
  try {
    history.value = await redeemAPI.getHistory()
  } catch (error) {
    console.error('Failed to fetch history:', error)
  } finally {
    loadingHistory.value = false
  }
}

const refreshRedeemData = async () => {
  await Promise.all([
    authStore.refreshUser().catch(error => {
      console.error('Failed to refresh user:', error)
    }),
    fetchHistory()
  ])
}

const handleRedeem = async () => {
  if (!redeemCode.value.trim()) {
    appStore.showError(t('redeem.pleaseEnterCode'))
    return
  }

  submitting.value = true
  errorMessage.value = ''
  redeemResult.value = null

  try {
    const result = await redeemAPI.redeem(redeemCode.value.trim())

    redeemResult.value = result

    // Refresh user data to get updated balance/concurrency
    await authStore.refreshUser()

    // If subscription type, immediately refresh subscription status
    if (result.type === 'subscription') {
      try {
        await subscriptionStore.fetchActiveSubscriptions(true) // force refresh
      } catch (error) {
        console.error('Failed to refresh subscriptions after redeem:', error)
        appStore.showWarning(t('redeem.subscriptionRefreshFailed'))
      }
    }

    // Clear the input
    redeemCode.value = ''

    // Refresh history
    await fetchHistory()

    // Show success toast
    appStore.showSuccess(t('redeem.codeRedeemSuccess'))
  } catch (error: any) {
    errorMessage.value = error.response?.data?.detail || t('redeem.failedToRedeem')

    appStore.showError(t('redeem.redeemFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  fetchHistory()
  try {
    const settings = await authAPI.getPublicSettings()
    contactInfo.value = settings.contact_info || ''
  } catch (error) {
    console.error('Failed to load contact info:', error)
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.redeem-page {
  width: min(100%, 112rem);
  --redeem-positive: var(--md-chart-5);
  --redeem-negative: var(--md-error);
  --redeem-subscription: var(--md-chart-3);
  --redeem-concurrency: var(--md-chart-1);
}

.redeem-workspace {
  display: grid;
  gap: 24px;
}

.redeem-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.redeem-header h1 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1.2;
}

.redeem-header p {
  margin: 10px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 1rem;
}

.redeem-icon-button,
.redeem-history-actions button {
  display: inline-flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease, opacity 160ms ease;
}

.redeem-icon-button:hover:not(:disabled),
.redeem-history-actions button:hover:not(:disabled) {
  background: var(--md-state-hover);
  color: var(--md-on-surface);
}

.redeem-icon-button:disabled,
.redeem-history-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.redeem-balance-card {
  display: flex;
  min-height: 104px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface-container);
  padding: 22px 24px;
  box-shadow: var(--md-elevation-1);
}

.redeem-balance-card > div:first-child > span {
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  font-weight: 650;
}

.redeem-balance-value {
  display: flex;
  margin-top: 8px;
  align-items: baseline;
  gap: 12px;
  color: var(--md-on-surface);
  font-variant-numeric: tabular-nums;
}

.redeem-balance-value span {
  color: var(--md-on-surface-variant);
  font-size: 2.25rem;
  font-weight: 500;
}

.redeem-balance-value strong {
  font-size: 2.75rem;
  font-weight: 750;
  line-height: 1;
}

.redeem-balance-meta {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 999px;
  background: var(--md-surface);
  padding: 8px 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  font-weight: 650;
}

.redeem-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 22px;
}

.redeem-panel {
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.redeem-panel-header {
  display: flex;
  min-height: 78px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--md-outline-variant);
  padding: 0 22px;
}

.redeem-panel-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 1.0625rem;
  font-weight: 700;
}

.redeem-panel-body {
  display: grid;
  gap: 18px;
  padding: 18px 22px 20px;
}

.redeem-form {
  display: grid;
  gap: 12px;
}

.redeem-code-field {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  overflow: hidden;
  min-height: 58px;
  border: 1px solid var(--md-outline);
  border-radius: 8px;
  background: var(--md-surface-container);
}

.redeem-code-field span {
  display: inline-flex;
  align-self: stretch;
  align-items: center;
  border-right: 1px solid var(--md-outline-variant);
  background: var(--md-surface);
  padding: 0 16px;
  color: var(--md-on-surface);
  font-size: 1rem;
  font-weight: 650;
}

.redeem-code-field input {
  width: 100%;
  min-width: 0;
  border: 0;
  background: transparent;
  padding: 0 16px;
  color: var(--md-on-surface);
  font-size: 1rem;
  font-weight: 500;
  letter-spacing: 0;
  outline: none;
  text-align: right;
}

.redeem-code-field input::placeholder {
  color: color-mix(in srgb, var(--md-on-surface-variant) 72%, transparent);
}

.redeem-code-field:focus-within {
  border-color: var(--md-primary);
  box-shadow: 0 0 0 2px var(--md-state-focus);
}

.redeem-field-hint,
.redeem-account-note {
  margin: 0;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  text-align: center;
}

.redeem-submit-button {
  display: inline-flex;
  min-height: 60px;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: color-mix(in srgb, var(--md-info) 82%, var(--md-primary));
  color: white;
  font-size: 1rem;
  font-weight: 750;
  transition: background-color 160ms ease, opacity 160ms ease;
}

.redeem-submit-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-info) 72%, var(--md-primary));
}

.redeem-submit-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.redeem-result-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 10px;
  background: var(--md-surface-container-low);
  padding: 14px;
}

.redeem-result-success {
  border-color: color-mix(in srgb, var(--redeem-positive) 32%, var(--md-outline-variant));
}

.redeem-result-error {
  border-color: color-mix(in srgb, var(--redeem-negative) 36%, var(--md-outline-variant));
}

.redeem-result-icon {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--md-surface);
  color: var(--md-on-surface);
}

.redeem-result-success .redeem-result-icon {
  color: var(--redeem-positive);
}

.redeem-result-error .redeem-result-icon {
  color: var(--redeem-negative);
}

.redeem-result-card h3,
.redeem-result-card p {
  margin: 0;
}

.redeem-result-card h3 {
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
}

.redeem-result-card p {
  margin-top: 4px;
  color: var(--md-on-surface-variant);
  font-size: 0.8125rem;
  line-height: 1.55;
}

.redeem-result-details {
  display: grid;
  gap: 2px;
  margin-top: 8px;
}

.redeem-result-details strong {
  color: var(--md-on-surface);
  font-weight: 700;
}

.redeem-rules-intro {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 12px;
  color: var(--md-on-surface-variant);
}

.redeem-rules-intro p {
  margin: 0;
  font-size: 0.875rem;
  line-height: 1.55;
}

.redeem-rule-list {
  display: grid;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.redeem-rule-list li {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}

.redeem-rule-list span {
  display: inline-flex;
  width: 40px;
  height: 30px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container);
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 750;
}

.redeem-rule-list p {
  margin: 5px 0 0;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  line-height: 1.55;
}

.redeem-contact-card {
  display: grid;
  gap: 6px;
  border-top: 1px solid var(--md-outline-variant);
  padding-top: 16px;
}

.redeem-contact-card span {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 650;
}

.redeem-contact-card strong {
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.redeem-history-section {
  display: grid;
  gap: 28px;
  margin-top: 32px;
}

.redeem-history-rule {
  height: 1px;
  background: var(--md-outline-variant);
}

.redeem-history-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.redeem-history-header h2 {
  margin: 0;
  color: var(--md-on-surface-variant);
  font-size: 1.125rem;
  font-weight: 700;
}

.redeem-history-header p {
  margin: 12px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
}

.redeem-history-actions {
  display: inline-flex;
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container);
}

.redeem-history-table {
  overflow: hidden;
  border: 1px solid var(--md-outline);
  border-radius: 8px;
  background: var(--md-surface);
}

.redeem-history-empty {
  display: grid;
  min-height: 52px;
  place-items: center;
  color: var(--md-on-surface-variant);
  font-size: 0.9375rem;
}

.redeem-history-list {
  display: grid;
}

.redeem-history-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--md-outline-variant);
  padding: 12px 16px;
}

.redeem-history-row:last-child {
  border-bottom: 0;
}

.redeem-history-main {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.redeem-history-marker {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--md-surface-container);
  color: var(--md-on-surface-variant);
}

.redeem-history-marker-positive {
  background: color-mix(in srgb, var(--redeem-positive) 14%, transparent);
  color: var(--redeem-positive);
}

.redeem-history-marker-negative {
  background: color-mix(in srgb, var(--redeem-negative) 14%, transparent);
  color: var(--redeem-negative);
}

.redeem-history-marker-subscription {
  background: color-mix(in srgb, var(--redeem-subscription) 14%, transparent);
  color: var(--redeem-subscription);
}

.redeem-history-marker-concurrency {
  background: color-mix(in srgb, var(--redeem-concurrency) 14%, transparent);
  color: var(--redeem-concurrency);
}

.redeem-history-main p {
  margin: 0;
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.redeem-history-main span,
.redeem-history-value span {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.redeem-history-value {
  display: grid;
  gap: 3px;
  flex: 0 0 auto;
  max-width: 260px;
  text-align: right;
}

.redeem-history-value strong {
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
}

.redeem-history-value-positive {
  color: var(--redeem-positive) !important;
}

.redeem-history-value-negative {
  color: var(--redeem-negative) !important;
}

.redeem-history-value-subscription {
  color: var(--redeem-subscription) !important;
}

.redeem-history-value-concurrency {
  color: var(--redeem-concurrency) !important;
}

.redeem-history-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
}

.redeem-history-notes {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1180px) {
  .redeem-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 720px) {
  .redeem-page {
    width: 100%;
  }

  .redeem-header,
  .redeem-balance-card,
  .redeem-history-header,
  .redeem-history-row {
    align-items: stretch;
    flex-direction: column;
  }

  .redeem-balance-value strong {
    font-size: 2rem;
  }

  .redeem-balance-value span {
    font-size: 1.75rem;
  }

  .redeem-balance-meta {
    width: fit-content;
  }

  .redeem-code-field {
    grid-template-columns: minmax(0, 1fr);
  }

  .redeem-code-field span {
    min-height: 42px;
    border-right: 0;
    border-bottom: 1px solid var(--md-outline-variant);
  }

  .redeem-code-field input {
    min-height: 54px;
    text-align: left;
  }

  .redeem-history-value {
    max-width: none;
    text-align: left;
  }
}
</style>
