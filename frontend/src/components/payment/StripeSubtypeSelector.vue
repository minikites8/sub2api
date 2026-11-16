<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MethodFeeRates } from '@/types/payment'
import { paymentFeeRange } from './fees'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import paymentIcon from '@/assets/icons/payment.svg'
import stripeIcon from '@/assets/icons/stripe.svg'

defineProps<{ methods: Record<string, MethodFeeRates> }>()
const selected = defineModel<string>({ required: true })
const { t } = useI18n()
const icons: Record<string, string> = { alipay: alipayIcon, wxpay: wxpayIcon, card: paymentIcon, link: stripeIcon }

function rateLabel(rates: MethodFeeRates): string {
  const { min, max } = paymentFeeRange(rates)
  return `${t('payment.fee')} ${min === max ? min : `${min}–${max}`}%`
}
</script>

<template>
  <fieldset class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-600">
    <legend class="px-1 text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('payment.stripePaymentMethod') }}
    </legend>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3" data-testid="stripe-subtype">
      <button
        v-for="(rates, method) in methods"
        :key="method"
        type="button"
        :data-testid="`stripe-method-${method}`"
        :aria-pressed="selected === method"
        :class="[
          'flex min-w-0 items-center justify-center gap-2 rounded-lg border px-3 py-3 transition-all focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500',
          selected === method
            ? 'border-indigo-500 bg-indigo-50 text-gray-900 shadow-sm dark:bg-indigo-950 dark:text-gray-100'
            : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200',
        ]"
        @click="selected = method"
      >
        <img :src="icons[method] || paymentIcon" alt="" class="h-6 w-6 shrink-0 object-contain" />
        <span class="flex min-w-0 flex-col items-start text-left">
          <span class="w-full truncate text-sm font-semibold">{{ t(`payment.methods.${method}`, method) }}</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ rateLabel(rates) }}</span>
        </span>
      </button>
    </div>
  </fieldset>
</template>
