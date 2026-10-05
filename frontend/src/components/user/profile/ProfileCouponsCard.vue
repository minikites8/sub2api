<template>
  <section data-testid="profile-coupons-card" class="card" aria-labelledby="profile-coupons-title">
    <div class="flex items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-6">
      <div>
        <h2 id="profile-coupons-title" class="text-lg font-medium text-gray-900 dark:text-white">{{ t('profile.coupons.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('profile.coupons.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loading" data-testid="refresh-coupons" @click="loadCoupons">
        {{ t('common.refresh') }}
      </button>
    </div>
    <div class="px-5 py-5 sm:px-6" :aria-busy="loading">
      <p v-if="loading" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400" role="status">{{ t('common.loading') }}</p>
      <div v-else-if="loadError" class="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300" role="alert">
        <span>{{ t('profile.coupons.loadFailed') }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadCoupons">{{ t('common.retry') }}</button>
      </div>
      <p v-else-if="coupons.length === 0" data-testid="coupons-empty" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('profile.coupons.empty') }}</p>
      <template v-else>
        <div class="grid gap-3 sm:grid-cols-2">
          <article v-for="coupon in coupons" :key="`${coupon.coupon_type}-${coupon.id}`" data-testid="profile-coupon" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t(`profile.coupons.types.${coupon.coupon_type}`) }}</span>
              <span :class="statusClasses[coupon.status]" class="rounded px-2 py-1 text-xs font-medium">{{ t(`profile.coupons.status.${coupon.status}`) }}</span>
            </div>
            <p class="mt-3 text-2xl font-semibold text-primary-600 dark:text-primary-400">{{ discountLabel(coupon.discount_percent) }}</p>
            <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ thresholdLabel(coupon) }}</p>
            <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('profile.coupons.remaining') }}</dt>
                <dd data-testid="coupon-remaining" class="mt-1 font-semibold text-gray-900 dark:text-white">{{ coupon.total_uses === 0 ? t('profile.coupons.unlimited') : t('profile.coupons.times', { count: coupon.remaining_uses }) }}</dd>
              </div>
              <div>
                <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('profile.coupons.usage') }}</dt>
                <dd class="mt-1 text-gray-700 dark:text-gray-200">{{ coupon.total_uses === 0 ? t('profile.coupons.times', { count: coupon.used_count }) : `${coupon.used_count} / ${coupon.total_uses}` }}</dd>
              </div>
            </dl>
            <div class="mt-4 flex flex-wrap items-center justify-between gap-2 text-xs">
              <span class="text-gray-500 dark:text-gray-400">{{ t('profile.coupons.issuedAt', { date: formatDate(coupon.created_at, { year: 'numeric', month: '2-digit', day: '2-digit' }, locale) }) }}</span>
              <RouterLink v-if="coupon.status === 'available'" :to="{ path: '/purchase', query: { tab: coupon.coupon_type === 'subscription' ? 'subscription' : 'recharge' } }" class="font-medium text-primary-600 hover:underline dark:text-primary-400">{{ t('profile.coupons.use') }}</RouterLink>
            </div>
          </article>
        </div>
        <p class="mt-4 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('profile.coupons.reservationHint') }}</p>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { paymentAPI } from '@/api/payment'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { UserDiscountCoupon } from '@/types/payment'
import { formatDate } from '@/utils/format'

const { t, locale } = useI18n()
const loading = ref(true)
const loadError = ref(false)
const coupons = ref<UserDiscountCoupon[]>([])
const statusClasses = {
  available: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  exhausted: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  inactive: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
}

function discountLabel(percent: number): string {
  return t('profile.coupons.discount', { rate: Number((percent / 10).toFixed(3)), percent })
}

function thresholdLabel(coupon: UserDiscountCoupon): string {
  if (coupon.min_amount <= 0) return t('profile.coupons.anyAmount')
  const currency = coupon.coupon_type === 'subscription' ? 'USD' : 'CNY'
  return t(`profile.coupons.threshold.${coupon.coupon_type}`, { amount: formatPaymentAmount(coupon.min_amount, currency, locale?.value) })
}

async function loadCoupons() {
  loading.value = true
  loadError.value = false
  try {
    const { data } = await paymentAPI.getMyCoupons()
    coupons.value = data
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

onMounted(loadCoupons)
</script>
