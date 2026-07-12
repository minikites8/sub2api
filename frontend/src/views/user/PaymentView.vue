<template>
  <AppLayout>
    <div class="purchase-page mx-auto space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <!-- Tab Switcher (hide during payment and subscription confirm) -->
        <div v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan" class="purchase-tabs">
          <button v-for="tab in tabs" :key="tab.key"
            class="purchase-tab-button"
            :class="{ 'purchase-tab-button-active': activeTab === tab.key }"
            @click="activeTab = tab.key">{{ tab.label }}</button>
        </div>
        <!-- Payment in progress (shared by recharge and subscription) -->
        <template v-if="paymentPhase === 'paying'">
          <PaymentStatusPanel
            :order-id="paymentState.orderId"
            :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt"
            :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl"
            :order-type="paymentState.orderType"
            :currency="paymentState.currency || selectedCurrency"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>
        <!-- Tab content (select phase) -->
        <template v-else>
          <!-- Top-up Tab -->
          <template v-if="activeTab === 'recharge'">
            <section class="credits-workspace">
              <header class="credits-header">
                <div>
                  <h1>{{ t('payment.creditsTitle') }}</h1>
                  <p>{{ t('payment.personalAccount', { account: accountDisplayName }) }}</p>
                </div>
                <button
                  type="button"
                  class="credits-icon-button"
                  :disabled="loadingRecentOrders"
                  :title="t('common.refresh')"
                  :aria-label="t('common.refresh')"
                  @click="refreshPurchaseData"
                >
                  <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingRecentOrders }" />
                </button>
              </header>

              <div class="credits-balance-card">
                <div class="credits-balance-value">
                  <span>$</span>
                  <strong>{{ currentBalanceText }}</strong>
                </div>
                <div class="credits-balance-info" :title="t('payment.currentBalance')">
                  <Icon name="infoCircle" size="sm" />
                </div>
              </div>
            </section>

            <div v-if="enabledMethods.length === 0" class="purchase-panel purchase-empty-panel">
              <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
            </div>
            <template v-else>
              <section class="credits-grid">
                <article class="purchase-panel buy-credits-panel">
                  <header class="purchase-panel-header">
                    <h2>{{ t('payment.buyCredits') }}</h2>
                    <span v-if="minimumAmountLabel" class="credits-minimum-badge">
                      {{ minimumAmountLabel }}
                    </span>
                  </header>

                  <div class="purchase-panel-body">
                    <label class="credits-amount-field">
                      <span>{{ t('payment.amountLabel') }}</span>
                      <input
                        type="text"
                        inputmode="decimal"
                        :value="amountInputText"
                        :placeholder="t('payment.enterAmount')"
                        @input="handleAmountInput"
                      />
                    </label>
                    <p v-if="amountError" class="credits-field-error">{{ amountError }}</p>

                    <div v-if="validAmount > 0" class="credits-summary">
                      <div class="credits-summary-row">
                        <span>{{ t('payment.paymentAmount') }}</span>
                        <strong>{{ formatSelectedPaymentAmount(validAmount) }}</strong>
                      </div>
                      <div v-if="availableRechargePromo" class="credits-promo-card">
                        <div class="credits-promo-topline">
                          <span>{{ t('payment.rechargePromo.available') }}</span>
                          <code>{{ availableRechargePromo.promo_code }}</code>
                        </div>
                        <p v-if="rechargeDiscountActive">
                          {{ t('payment.rechargePromo.discountPreview', { discount: formatDiscountRate(rechargeDiscountPercent) }) }}
                          <span v-if="availableRechargePromo.discount_times === 0">{{ t('payment.rechargePromo.unlimitedDiscount') }}</span>
                          <span v-else>{{ t('payment.rechargePromo.remainingDiscount', { remaining: availableRechargePromo.discount_remaining }) }}</span>
                        </p>
                        <p v-if="rechargeBonusAmount > 0">
                          {{ t('payment.rechargePromo.bonusPreview', { amount: formatBalanceAmount(rechargeBonusAmount) }) }}
                        </p>
                      </div>
                      <div v-if="rechargeDiscountActive" class="credits-summary-row">
                        <span>{{ t('payment.rechargePromo.discountDeduction') }}</span>
                        <strong>-{{ formatSelectedPaymentAmount(rechargeDiscountAmount) }}</strong>
                      </div>
                      <div v-if="rechargeDiscountActive" class="credits-summary-row">
                        <span>{{ t('payment.rechargePromo.discountedPaymentAmount') }}</span>
                        <strong>{{ formatSelectedPaymentAmount(discountedRechargePaymentAmount) }}</strong>
                      </div>
                      <div v-if="feeRate > 0" class="credits-summary-row">
                        <span>{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                        <strong>{{ formatSelectedPaymentAmount(feeAmount) }}</strong>
                      </div>
                      <div class="credits-summary-row credits-summary-total">
                        <span>{{ t('payment.totalDue') }}</span>
                        <strong>{{ formatSelectedPaymentAmount(totalAmount) }}</strong>
                      </div>
                      <div v-if="showCreditedAmount" class="credits-summary-row">
                        <span>{{ t('payment.creditedBalance') }}</span>
                        <strong>{{ formatBalanceAmount(creditedAmount) }}</strong>
                      </div>
                      <p v-if="balanceRechargeMultiplier !== 1" class="credits-summary-note">
                        {{ t('payment.rechargeRatePreview', { usd: balanceRechargeMultiplier.toFixed(2) }) }}
                      </p>
                    </div>

                    <button type="button" class="credits-purchase-button" :disabled="!canSubmit || submitting" @click="handleSubmitRecharge">
                      <span v-if="submitting" class="flex items-center justify-center gap-2">
                        <span class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
                        {{ t('common.processing') }}
                      </span>
                      <span v-else>{{ t('payment.purchase') }}</span>
                    </button>

                    <div class="credits-account-note">
                      {{ t('payment.personalAccount', { account: accountDisplayName }) }}
                    </div>
                    <p class="credits-confirm-note">
                      {{ t('payment.confirmationHint') }}
                      <Icon name="infoCircle" size="xs" />
                    </p>

                    <footer class="credits-panel-footer">
                      <button type="button" @click="router.push('/orders')">{{ t('payment.viewUsage') }}</button>
                      <button type="button" @click="router.push('/redeem')">{{ t('payment.redeemPromoCode') }}</button>
                    </footer>
                  </div>
                </article>

                <article class="purchase-panel payment-method-panel">
                  <header class="purchase-panel-header">
                    <h2>{{ t('payment.paymentMethod') }}</h2>
                  </header>
                  <div class="purchase-panel-body">
                    <PaymentMethodSelector
                      :methods="methodOptions"
                      :selected="selectedMethod"
                      @select="selectedMethod = $event"
                    />
                  </div>
                </article>
              </section>

              <section class="recent-transactions">
                <div class="recent-transactions-rule"></div>
                <header class="recent-transactions-header">
                  <div>
                    <h2>{{ t('payment.recentTransactions') }}</h2>
                    <button type="button" class="enterprise-billing-contact" @click="showEnterpriseBillingContact = true">
                      {{ t('payment.enterpriseBilling') }}
                    </button>
                  </div>
                  <div class="recent-transactions-actions">
                    <button type="button" :disabled="loadingRecentOrders" @click="loadRecentOrders">
                      <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingRecentOrders }" />
                    </button>
                    <button type="button" @click="router.push('/orders')">
                      <Icon name="chevronRight" size="sm" />
                    </button>
                  </div>
                </header>

                <div class="recent-transactions-table">
                  <div v-if="loadingRecentOrders" class="recent-transactions-empty">
                    {{ t('common.processing') }}
                  </div>
                  <div v-else-if="recentOrders.length === 0" class="recent-transactions-empty">
                    {{ t('payment.noResults') }}
                  </div>
                  <div v-else class="recent-transaction-list">
                    <div v-for="order in recentOrders" :key="order.id" class="recent-transaction-row">
                      <div class="min-w-0">
                        <p>{{ formatRecentOrderTitle(order) }}</p>
                        <span>{{ formatRecentOrderMeta(order) }}</span>
                      </div>
                      <div class="recent-transaction-amount">
                        <strong>{{ formatOrderPayAmount(order) }}</strong>
                        <span>{{ t(`payment.status.${order.status.toLowerCase()}`, order.status) }}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </section>
            </template>
          </template>
          <!-- Subscribe Tab -->
          <template v-else-if="activeTab === 'subscription'">
            <!-- Subscription confirm (inline, replaces plan list) -->
            <template v-if="selectedPlan">
              <div class="card p-5">
                <!-- Header: platform badge + plan name -->
                <div class="mb-3 flex flex-wrap items-center gap-2">
                  <span :class="['rounded-md border px-2 py-0.5 text-xs font-medium', planBadgeClass]">
                    {{ platformLabel(selectedPlan.group_platform || '') }}
                  </span>
                  <h3 class="text-lg font-bold text-gray-900 dark:text-white">{{ selectedPlan.name }}</h3>
                </div>
                <!-- Price -->
                <div class="flex items-baseline gap-2">
                  <span v-if="selectedPlan.original_price" class="text-sm text-gray-400 line-through dark:text-gray-500">
                    {{ formatSelectedSubscriptionPaymentAmount(selectedPlan.original_price) }}
                  </span>
                  <span :class="['text-3xl font-bold', planTextClass]">{{ formatSelectedSubscriptionPaymentAmount(selectedPlan.price) }}</span>
                  <span class="text-sm text-gray-500 dark:text-gray-400">/ {{ planValiditySuffix }}</span>
                </div>
                <!-- Description -->
                <p v-if="selectedPlan.description" class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-gray-400">
                  {{ selectedPlan.description }}
                </p>
                <!-- Rate + Limits grid -->
                <div class="mt-3 grid grid-cols-2 gap-3">
                  <div>
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.rate') }}</span>
                    <div class="flex items-baseline">
                      <span :class="['text-lg font-bold', planTextClass]">×{{ selectedPlan.rate_multiplier ?? 1 }}</span>
                    </div>
                  </div>
                  <div v-if="planHasPeakRate(selectedPlan)">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.peakRate') }}</span>
                    <div class="text-sm font-semibold text-amber-700 dark:text-amber-300">
                      {{ planPeakRateLabel(selectedPlan) }}
                    </div>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.dailyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.daily_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.weekly_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.weeklyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.weekly_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.monthly_limit_usd != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.monthlyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ selectedPlan.monthly_limit_usd }}</div>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd == null && selectedPlan.weekly_limit_usd == null && selectedPlan.monthly_limit_usd == null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.quota') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">{{ t('payment.planCard.unlimited') }}</div>
                  </div>
                </div>
              </div>
              <div v-if="enabledMethods.length >= 1" class="card p-6">
                <PaymentMethodSelector
                  :methods="subMethodOptions"
                  :selected="selectedMethod"
                  @select="selectedMethod = $event"
                />
              </div>
              <div v-if="feeRate > 0 && selectedPlan.price > 0" class="card p-6">
                <div class="space-y-2 text-sm">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.amountLabel') }}</span>
                    <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(subPaymentAmount) }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                    <span class="text-gray-900 dark:text-white">{{ formatSelectedPaymentAmount(subFeeAmount) }}</span>
                  </div>
                  <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                    <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.actualPay') }}</span>
                    <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
                  </div>
                </div>
              </div>
              <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmitSubscription || submitting" @click="confirmSubscribe">
                <span v-if="submitting" class="flex items-center justify-center gap-2">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                  {{ t('common.processing') }}
                </span>
                <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
              </button>
              <button class="btn btn-secondary w-full" @click="selectedPlan = null">{{ t('common.cancel') }}</button>
            </template>
            <!-- Plan list -->
            <template v-else>
              <div v-if="checkout.plans.length === 0" class="card py-16 text-center">
                <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
                <p class="text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</p>
              </div>
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard v-for="plan in checkout.plans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlan" />
              </div>
              <!-- Active subscriptions (compact, below plan list) -->
              <div v-if="activeSubscriptions.length > 0">
                <p class="mb-2 text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.activeSubscription') }}</p>
                <div class="space-y-2">
                  <div v-for="sub in activeSubscriptions" :key="sub.id"
                    class="flex items-center gap-3 rounded-xl border border-gray-100 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
                    <div :class="['h-6 w-1 shrink-0 rounded-full', platformAccentBarClass(sub.group?.platform || '')]" />
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <span class="truncate text-xs font-semibold text-gray-900 dark:text-white">{{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}</span>
                        <span :class="['shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium', platformBadgeLightClass(sub.group?.platform || '')]">{{ platformLabel(sub.group?.platform || '') }}</span>
                      </div>
                      <div class="flex flex-wrap gap-x-3 text-[11px] text-gray-400 dark:text-gray-500">
                        <span>{{ t('payment.planCard.rate') }}: ×{{ sub.group?.rate_multiplier ?? 1 }}</span>
                        <span v-if="subscriptionHasPeakRate(sub)">{{ t('payment.planCard.peakRate') }}: {{ subscriptionPeakRateLabel(sub) }}</span>
                        <span v-if="sub.group?.daily_limit_usd == null && sub.group?.weekly_limit_usd == null && sub.group?.monthly_limit_usd == null">{{ t('payment.planCard.quota') }}: {{ t('payment.planCard.unlimited') }}</span>
                        <span v-if="sub.expires_at">{{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}</span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </div>
                    </div>
                    <span class="badge badge-success shrink-0 text-[10px]">{{ t('userSubscriptions.status.active') }}</span>
                  </div>
                </div>
              </div>
            </template>
          </template>
        </template>
        <div v-if="(checkout.help_text || checkout.help_image_url) && paymentPhase === 'select' && !selectedPlan" class="card p-4">
          <div class="flex flex-col items-center gap-3">
            <img v-if="checkout.help_image_url" :src="checkout.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer rounded-lg object-contain transition-opacity hover:opacity-80"
              @click="previewImage = checkout.help_image_url" />
            <p v-if="checkout.help_text" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ checkout.help_text }}</p>
          </div>
        </div>
      </template>
    </div>
    <BaseDialog
      :show="showEnterpriseBillingContact"
      :title="t('payment.enterpriseBillingContactTitle')"
      width="narrow"
      @close="showEnterpriseBillingContact = false"
    >
      <div class="enterprise-billing-contact-dialog">
        <template v-if="enterpriseBillingContactQr">
          <img
            :src="enterpriseBillingContactQr"
            :alt="t('payment.enterpriseBillingContactTitle')"
            class="enterprise-billing-contact-qr"
          />
          <p>{{ t('payment.enterpriseBillingContactHint') }}</p>
        </template>
        <template v-else>
          <p class="enterprise-billing-contact-unavailable">{{ t('payment.enterpriseBillingContactUnavailable') }}</p>
          <strong v-if="appStore.contactInfo">{{ appStore.contactInfo }}</strong>
        </template>
      </div>
    </BaseDialog>
    <!-- Renewal Plan Selection Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showRenewalModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" @click.self="closeRenewalModal">
          <div class="relative w-full max-w-lg rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <!-- Close button -->
            <button class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="closeRenewalModal">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.selectPlan') }}</h3>
            <div class="space-y-4">
              <SubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlanFromModal" />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
    <!-- Image Preview Overlay -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl" />
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel, type PeakRateFields } from '@/utils/peak-rate'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType, PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, getPaymentPopupFeatures, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from '@/components/payment/providerConfig'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformAccentBarClass, platformBadgeLightClass, platformBadgeClass, platformTextClass, platformLabel } from '@/utils/platformColors'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

function getDaysRemaining(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now()
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
}

function subscriptionHasPeakRate(sub: { group?: PeakRateFields | null }): boolean {
  return hasPeakRate(sub.group)
}

function subscriptionPeakRateLabel(sub: { group?: PeakRateFields | null }): string {
  return formatPeakRateWindow(sub.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const amountInputText = ref('')
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')
const recentOrders = ref<PaymentOrder[]>([])
const loadingRecentOrders = ref(false)
const showEnterpriseBillingContact = ref(false)
const enterpriseBillingContactQr = computed(() => appStore.enterpriseBillingContactQr)

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}

interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    intentId: '',
    currency: '',
    countryCode: '',
    paymentEnv: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  removeRecoverySnapshot()
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number },
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || typeof window === 'undefined') {
    return normalizedUrl
  }

  try {
    const targetUrl = new URL(normalizedUrl, window.location.origin)
    const redirectPath = targetUrl.searchParams.get('redirect') || '/purchase'
    const redirectUrl = new URL(redirectPath, window.location.origin)
    const paymentType = normalizeVisibleMethod(context.paymentType) || context.paymentType.trim() || 'wxpay'

    redirectUrl.searchParams.set('payment_type', paymentType)
    redirectUrl.searchParams.set('order_type', context.orderType)

    if (context.planId) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return targetUrl.toString()
  } catch {
    return normalizedUrl
  }
}

function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSuccess() {
  removeRecoverySnapshot()
  authStore.refreshUser()
  loadRecentOrders()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  return result
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
})
// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})

const availableRechargePromo = computed(() => checkout.value.first_recharge_promo)
const rechargeBonusAmount = computed(() => {
  const bonus = Number(availableRechargePromo.value?.bonus_amount || 0)
  return Number.isFinite(bonus) && bonus > 0 ? bonus : 0
})
const rechargeDiscountPercent = computed(() => {
  const discount = Number(availableRechargePromo.value?.discount_percent || 0)
  return Number.isFinite(discount) ? Math.min(Math.max(discount, 0), 100) : 0
})
const rechargeDiscountActive = computed(() =>
  availableRechargePromo.value?.discount_set === true
    && rechargeDiscountPercent.value > 0
    && rechargeDiscountPercent.value < 100
)
const discountedRechargePaymentAmount = computed(() => {
  if (!rechargeDiscountActive.value || validAmount.value <= 0) return validAmount.value
  return Math.round((validAmount.value * (rechargeDiscountPercent.value / 100)) * 100) / 100
})
const rechargeDiscountAmount = computed(() =>
  Math.max(0, Math.round((validAmount.value - discountedRechargePaymentAmount.value) * 100) / 100)
)
const creditedAmount = computed(() =>
  Math.round(((validAmount.value * balanceRechargeMultiplier.value) + rechargeBonusAmount.value) * 100) / 100
)
const showCreditedAmount = computed(() => balanceRechargeMultiplier.value !== 1 || rechargeBonusAmount.value > 0)

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => limit.single_min))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
const accountDisplayName = computed(() => user.value?.email || user.value?.username || '-')
const currentBalanceText = computed(() => (Number(user.value?.balance || 0)).toFixed(2))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function currencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function roundPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.round(value * factor) / factor
}

function ceilPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.ceil(value * factor) / factor
}

function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  const rate = subscriptionUsdToCnyRate.value
  if (rate <= 0 || currency !== DEFAULT_PAYMENT_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

function formatBalanceAmount(value: number): string {
  return `$${(Number.isFinite(value) ? value : 0).toFixed(2)}`
}

function formatCompactPaymentAmount(value: number): string {
  const amountText = Number.isInteger(value)
    ? String(value)
    : value.toFixed(2).replace(/\.?0+$/, '')

  switch (selectedCurrency.value) {
    case 'CNY':
    case 'JPY':
      return `${amountText}¥`
    case 'USD':
      return `$${amountText}`
    case 'HKD':
      return `HK$${amountText}`
    default:
      return `${amountText} ${selectedCurrency.value}`
  }
}

const minimumAmountLabel = computed(() => {
  if (globalMinAmount.value <= 0) return ''
  return t('payment.minimumAmount', {
    amount: formatCompactPaymentAmount(globalMinAmount.value)
  })
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function handleAmountInput(event: Event) {
  const input = event.target as HTMLInputElement
  const nextValue = input.value.trim()
  if (!AMOUNT_PATTERN.test(nextValue)) {
    input.value = amountInputText.value
    return
  }

  amountInputText.value = nextValue
  if (!nextValue) {
    amount.value = null
    return
  }

  const parsed = Number.parseFloat(nextValue)
  amount.value = Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

watch(amount, (nextAmount) => {
  const nextText = nextAmount == null ? '' : String(nextAmount)
  if (nextText !== amountInputText.value) {
    amountInputText.value = nextText
  }
}, { immediate: true })

async function loadRecentOrders() {
  loadingRecentOrders.value = true
  try {
    const response = await paymentAPI.getMyOrders({ page: 1, page_size: 5 })
    recentOrders.value = response.data.items || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loadingRecentOrders.value = false
  }
}

async function refreshPurchaseData() {
  await Promise.all([
    authStore.refreshUser().catch(() => {}),
    loadRecentOrders()
  ])
}

function formatOrderPayAmount(order: PaymentOrder): string {
  return formatPaymentAmount(order.pay_amount || order.amount || 0, normalizePaymentCurrency(order.currency), localeCode.value)
}

function formatRecentOrderTitle(order: PaymentOrder): string {
  const method = t(`payment.methods.${order.payment_type}`, order.payment_type)
  const typeKey = order.order_type === 'subscription' ? 'payment.tabSubscribe' : 'payment.tabTopUp'
  return `${t(typeKey)} · ${method}`
}

function formatRecentOrderMeta(order: PaymentOrder): string {
  return `#${order.id} · ${new Date(order.created_at).toLocaleString()}`
}

function formatSelectedSubscriptionPaymentAmount(value: number): string {
  return formatSelectedPaymentAmount(subscriptionPaymentAmountForCurrency(value, selectedCurrency.value))
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
const feeAmount = computed(() =>
  feeRate.value > 0 && discountedRechargePaymentAmount.value > 0
    ? Math.ceil(((discountedRechargePaymentAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() =>
  validAmount.value > 0
    ? Math.round((discountedRechargePaymentAmount.value + feeAmount.value) * 100) / 100
    : 0
)
function formatDiscountRate(value: number): string {
  if (!Number.isFinite(value)) return '0'
  return Number((value / 10).toFixed(2)).toString()
}

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(validAmount.value, m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can)
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && validAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && validAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

const subPaymentAmount = computed(() => {
  const price = selectedPlan.value?.price ?? 0
  return subscriptionPaymentAmountForCurrency(price, selectedCurrency.value)
})

const subFeeAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return 0
  return ceilPaymentAmount((subPaymentAmount.value * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return subPaymentAmount.value
  return roundPaymentAmount(subPaymentAmount.value + subFeeAmount.value, selectedCurrency.value)
})

function subscriptionTotalAmountForCurrency(value: number, currency: string): number {
  const paymentAmount = subscriptionPaymentAmountForCurrency(value, currency)
  if (feeRate.value <= 0 || paymentAmount <= 0) return paymentAmount
  const fee = ceilPaymentAmount((paymentAmount * feeRate.value) / 100, currency)
  return roundPaymentAmount(paymentAmount + fee, currency)
}

// Subscription-specific: method options based on gateway pay amount
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const price = selectedPlan.value?.price ?? 0
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const currency = normalizePaymentCurrency(ml?.currency)
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(subscriptionTotalAmountForCurrency(price, currency), type),
    }
  })
})

const canSubmitSubscription = computed(() =>
  selectedPlan.value !== null
    && amountFitsMethod(subTotalAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

// Payment button class: follows selected payment method color
const paymentButtonClass = computed(() => {
  const m = selectedMethod.value
  if (!m) return 'btn-primary'
  if (isBuiltInAlipayMethod(m)) return 'btn-alipay'
  if (isBuiltInWxpayMethod(m)) return 'btn-wxpay'
  if (m === 'stripe') return 'btn-stripe'
  if (m === 'airwallex') return 'btn-airwallex'
  return 'btn-primary'
})

// Subscription confirm: platform accent colors (clean card, no gradient)
const planBadgeClass = computed(() => platformBadgeClass(selectedPlan.value?.group_platform || ''))
const planTextClass = computed(() => platformTextClass(selectedPlan.value?.group_platform || ''))

// Renewal modal state
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

const planValiditySuffix = computed(() => {
  if (!selectedPlan.value) return ''
  const u = selectedPlan.value.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${selectedPlan.value.validity_days}${t('payment.days')}`
})

function planHasPeakRate(plan: SubscriptionPlan): boolean {
  return hasPeakRate(plan)
}

function planPeakRateLabel(plan: SubscriptionPlan): string {
  return formatPeakRateWindow(plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

function selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
}

function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  errorMessage.value = ''
}

function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  await createOrder(validAmount.value, 'balance')
}

async function confirmSubscribe() {
  if (!selectedPlan.value || submitting.value) return
  await createOrder(selectedPlan.value.price, 'subscription', selectedPlan.value.id)
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({
        path: '/payment/airwallex',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && visibleMethod === 'alipay'),
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, {
        paymentType: visibleMethod,
        orderType,
        planId,
        orderAmount,
      })
      return
    }

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'airwallex_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
          || (visibleMethods.value[restored.paymentType] ? restored.paymentType : '')
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    loadRecentOrders()
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    // Handle renewal navigation: ?tab=subscription&group=123
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) {
          selectedPlan.value = groupPlans[0]
        } else if (groupPlans.length > 1) {
          renewGroupId.value = groupId
          showRenewalModal.value = true
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>

<style scoped>
.purchase-page {
  width: min(100%, 112rem);
}

.purchase-tabs {
  display: inline-flex;
  gap: 4px;
  border: 1px solid var(--md-outline-variant);
  border-radius: 10px;
  background: var(--md-surface-container-low);
  padding: 4px;
}

.purchase-tab-button {
  min-width: 7rem;
  border-radius: 8px;
  padding: 0.625rem 1rem;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  font-weight: 600;
  transition: background-color 160ms ease, color 160ms ease;
}

.purchase-tab-button:hover {
  background: var(--md-state-hover);
  color: var(--md-on-surface);
}

.purchase-tab-button-active {
  background: var(--md-surface);
  color: var(--md-on-surface);
  box-shadow: var(--md-elevation-1);
}

.credits-workspace {
  display: grid;
  gap: 24px;
}

.credits-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.credits-header h1 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 1.5rem;
  font-weight: 700;
  line-height: 1.2;
}

.credits-header p {
  margin: 10px 0 0;
  color: var(--md-on-surface-variant);
  font-size: 1rem;
}

.credits-icon-button,
.recent-transactions-actions button {
  display: inline-flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--md-on-surface-variant);
  transition: background-color 160ms ease, color 160ms ease;
}

.credits-icon-button:hover:not(:disabled),
.recent-transactions-actions button:hover:not(:disabled) {
  background: var(--md-state-hover);
  color: var(--md-on-surface);
}

.credits-icon-button:disabled,
.recent-transactions-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.credits-balance-card {
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

.credits-balance-value {
  display: flex;
  align-items: baseline;
  gap: 12px;
  color: var(--md-on-surface);
  font-variant-numeric: tabular-nums;
}

.credits-balance-value span {
  color: var(--md-on-surface-variant);
  font-size: 2.25rem;
  font-weight: 500;
}

.credits-balance-value strong {
  font-size: 2.75rem;
  font-weight: 750;
  line-height: 1;
}

.credits-balance-info {
  color: var(--md-on-surface-variant);
}

.credits-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 22px;
}

.purchase-panel {
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  box-shadow: var(--md-elevation-1);
}

.purchase-empty-panel {
  display: grid;
  min-height: 280px;
  place-items: center;
}

.purchase-panel-header {
  display: flex;
  min-height: 78px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--md-outline-variant);
  padding: 0 22px;
}

.purchase-panel-header h2 {
  margin: 0;
  color: var(--md-on-surface);
  font-size: 1.0625rem;
  font-weight: 700;
}

.credits-minimum-badge {
  flex: 0 0 auto;
  border: 1px solid var(--md-outline-variant);
  border-radius: 999px;
  background: var(--md-surface-container);
  padding: 6px 10px;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
  font-weight: 650;
  line-height: 1;
}

.purchase-panel-body {
  display: grid;
  gap: 20px;
  padding: 18px 22px 20px;
}

.buy-credits-panel .purchase-panel-body {
  gap: 16px;
}

.credits-amount-field {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  overflow: hidden;
  min-height: 58px;
  border: 1px solid var(--md-outline);
  border-radius: 8px;
  background: var(--md-surface-container);
}

.credits-amount-field span {
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

.credits-amount-field input {
  width: 100%;
  min-width: 0;
  border: 0;
  background: transparent;
  padding: 0 16px;
  color: var(--md-on-surface);
  font-size: 1.25rem;
  font-weight: 500;
  text-align: right;
  outline: none;
}

.credits-field-error {
  margin: -6px 0 0;
  color: var(--md-warning);
  font-size: 0.75rem;
}

.credits-summary {
  display: grid;
  gap: 10px;
}

.credits-summary-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
}

.credits-summary-row strong {
  color: var(--md-on-surface);
  font-weight: 700;
}

.credits-summary-total {
  border-top: 1px solid var(--md-outline-variant);
  padding-top: 10px;
}

.credits-promo-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container-low);
  padding: 10px 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.credits-promo-card p {
  margin: 6px 0 0;
}

.credits-promo-topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: var(--md-on-surface);
  font-weight: 650;
}

.credits-promo-topline code {
  border-radius: 6px;
  background: var(--md-surface);
  padding: 2px 8px;
  color: var(--md-on-surface-variant);
  font-size: 0.6875rem;
}

.credits-summary-note,
.credits-confirm-note,
.credits-account-note {
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  text-align: center;
}

.credits-summary-note {
  border-top: 1px solid var(--md-outline-variant);
  margin: 0;
  padding-top: 10px;
  text-align: left;
}

.credits-purchase-button {
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

.credits-purchase-button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--md-info) 72%, var(--md-primary));
}

.credits-purchase-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.credits-confirm-note {
  display: inline-flex;
  justify-content: center;
  gap: 6px;
  margin: 4px 0 0;
  font-size: 0.75rem;
}

.credits-panel-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-top: 4px;
}

.credits-panel-footer button {
  color: var(--md-info);
  font-size: 0.875rem;
  font-weight: 650;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.payment-method-panel :deep(label) {
  display: none;
}

.payment-method-panel :deep(.grid) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.payment-method-panel :deep(button) {
  min-height: 58px;
  border-radius: 8px;
  background: var(--md-surface-container);
}

.payment-method-panel :deep(button.shadow-sm) {
  border-color: var(--md-outline);
  background: var(--md-surface-container-high);
  color: var(--md-on-surface);
  box-shadow: inset 0 0 0 1px var(--md-outline);
}

.recent-transactions {
  display: grid;
  gap: 28px;
  margin-top: 32px;
}

.recent-transactions-rule {
  height: 1px;
  background: var(--md-outline-variant);
}

.recent-transactions-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.recent-transactions-header h2 {
  margin: 0;
  color: var(--md-on-surface-variant);
  font-size: 1.125rem;
  font-weight: 700;
}

.enterprise-billing-contact {
  display: block;
  margin-top: 12px;
  color: var(--md-on-surface-variant);
  font-size: 0.875rem;
  text-align: left;
  text-decoration: underline;
  text-underline-offset: 3px;
  transition: color 160ms ease;
}

.enterprise-billing-contact:hover,
.enterprise-billing-contact:focus-visible {
  color: var(--md-primary);
  outline: none;
}

.enterprise-billing-contact-dialog {
  display: grid;
  justify-items: center;
  gap: 16px;
  color: var(--md-on-surface-variant);
  text-align: center;
}

.enterprise-billing-contact-dialog p {
  margin: 0;
  font-size: 0.875rem;
}

.enterprise-billing-contact-qr {
  width: min(100%, 256px);
  aspect-ratio: 1;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: #fff;
  object-fit: contain;
}

.enterprise-billing-contact-unavailable {
  color: var(--md-on-surface);
  font-weight: 600;
}

.recent-transactions-actions {
  display: inline-flex;
  overflow: hidden;
  border: 1px solid var(--md-outline-variant);
  border-radius: 8px;
  background: var(--md-surface-container);
}

.recent-transactions-actions button {
  border-radius: 0;
}

.recent-transactions-actions button + button {
  border-left: 1px solid var(--md-outline-variant);
}

.recent-transactions-table {
  overflow: hidden;
  border: 1px solid var(--md-outline);
  border-radius: 8px;
  background: var(--md-surface);
}

.recent-transactions-empty {
  display: grid;
  min-height: 52px;
  place-items: center;
  color: var(--md-on-surface-variant);
  font-size: 0.9375rem;
}

.recent-transaction-list {
  display: grid;
}

.recent-transaction-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--md-outline-variant);
  padding: 12px 16px;
}

.recent-transaction-row:last-child {
  border-bottom: 0;
}

.recent-transaction-row p {
  margin: 0;
  overflow: hidden;
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-transaction-row span {
  color: var(--md-on-surface-variant);
  font-size: 0.75rem;
}

.recent-transaction-amount {
  display: grid;
  gap: 3px;
  flex: 0 0 auto;
  text-align: right;
}

.recent-transaction-amount strong {
  color: var(--md-on-surface);
  font-size: 0.875rem;
  font-weight: 700;
}

@media (max-width: 1180px) {
  .credits-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 720px) {
  .purchase-page {
    width: 100%;
  }

  .purchase-tabs {
    width: 100%;
  }

  .purchase-tab-button {
    min-width: 0;
    flex: 1;
  }

  .credits-header,
  .recent-transactions-header,
  .credits-panel-footer,
  .recent-transaction-row {
    align-items: stretch;
    flex-direction: column;
  }

  .credits-balance-card {
    min-height: 92px;
    padding: 18px;
  }

  .credits-balance-value strong {
    font-size: 2rem;
  }

  .credits-balance-value span {
    font-size: 1.75rem;
  }

  .payment-method-panel :deep(.grid) {
    grid-template-columns: minmax(0, 1fr);
  }

  .recent-transaction-amount {
    text-align: left;
  }
}
</style>
