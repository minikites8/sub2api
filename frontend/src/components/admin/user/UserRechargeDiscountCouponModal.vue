<template>
  <BaseDialog :show="show" :title="t('admin.users.rechargeCoupon.title')" width="wide" @close="emit('close')">
    <div v-if="user" class="space-y-6">
      <div class="flex items-center gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
          <Icon name="gift" size="md" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-gray-900 dark:text-gray-100">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ couponSummary }}</p>
        </div>
      </div>

      <section>
        <div class="mb-3 flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.users.rechargeCoupon.issuedTitle') }}</h3>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.users.rechargeCoupon.issuedCount', { count: coupons.length }) }}</span>
        </div>

        <div v-if="loading" class="flex min-h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="loadError" class="flex min-h-32 flex-col items-center justify-center gap-3 rounded-lg border border-red-200 bg-red-50 p-5 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
          <span>{{ t('admin.users.rechargeCoupon.loadFailed') }}</span>
          <button type="button" class="btn btn-secondary" @click="loadCoupons">
            <Icon name="refresh" size="sm" />
            {{ t('common.retry') }}
          </button>
        </div>
        <div v-else-if="coupons.length === 0" class="flex min-h-32 items-center justify-center rounded-lg border border-dashed border-gray-300 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.users.rechargeCoupon.empty') }}
        </div>
        <div v-else class="grid max-h-72 grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2">
          <article v-for="coupon in coupons" :key="coupon.id" data-test="coupon-item" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex items-start justify-between gap-3">
              <div>
                <p class="font-semibold text-gray-900 dark:text-gray-100">
                  {{ couponRuleLabel(coupon) }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ coupon.id }} · {{ formatDateTime(coupon.created_at) }}</p>
                <p class="mt-1 text-xs font-medium text-gray-600 dark:text-gray-300">
                  {{ couponSourceLabel(coupon) }}
                </p>
              </div>
              <span :class="couponStatusClass(coupon)" class="shrink-0 rounded px-2 py-1 text-xs font-medium">
                {{ couponStatusLabel(coupon) }}
              </span>
            </div>
            <div class="mt-3 flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.rechargeCoupon.usage') }}</span>
              <strong class="text-gray-900 dark:text-gray-100">
                {{ coupon.total_uses === 0 ? t('admin.users.rechargeCoupon.unlimitedUsage', { used: coupon.used_count }) : `${coupon.used_count} / ${coupon.total_uses}` }}
              </strong>
            </div>
            <p v-if="coupon.notes" class="mt-2 break-words text-sm text-gray-600 dark:text-gray-300">{{ coupon.notes }}</p>
          </article>
        </div>
      </section>

      <form id="recharge-coupon-form" class="space-y-5 border-t border-gray-200 pt-5 dark:border-dark-600" @submit.prevent="handleSubmit">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.users.rechargeCoupon.issueTitle') }}</h3>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="input-label" for="coupon-min-amount">{{ t('admin.users.rechargeCoupon.minAmount') }}</label>
            <div class="relative">
              <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">¥</span>
              <input id="coupon-min-amount" v-model.number="form.minAmount" class="input pl-8" type="number" min="0.01" step="0.01" required />
            </div>
          </div>
          <div>
            <label class="input-label" for="coupon-discount-rate">{{ t('admin.users.rechargeCoupon.discountRate') }}</label>
            <div class="relative">
              <input id="coupon-discount-rate" v-model.number="form.discountRate" class="input pr-8" type="number" min="0.01" max="9.99" step="0.01" required />
              <span class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500">{{ t('admin.users.rechargeCoupon.rateUnit') }}</span>
            </div>
          </div>
          <div>
            <label class="input-label" for="coupon-total-uses">{{ t('admin.users.rechargeCoupon.totalUses') }}</label>
            <input id="coupon-total-uses" v-model.number="form.totalUses" class="input" type="number" min="1" step="1" required />
          </div>
        </div>

        <div>
          <label class="input-label" for="coupon-notes">{{ t('admin.users.notes') }}</label>
          <textarea id="coupon-notes" v-model="form.notes" class="input" rows="2" :placeholder="t('admin.users.rechargeCoupon.notesPlaceholder')"></textarea>
        </div>
      </form>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="submit" form="recharge-coupon-form" class="btn bg-emerald-600 text-white hover:bg-emerald-700" :disabled="submitting || !isValid">
          {{ submitting ? t('common.saving') : t('admin.users.rechargeCoupon.issue') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminUser } from '@/types'
import type { RechargeDiscountCoupon } from '@/api/admin/users'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits<{ close: []; success: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)
const loading = ref(false)
const loadError = ref(false)
const coupons = ref<RechargeDiscountCoupon[]>([])
const form = reactive({ minAmount: 100, discountRate: 8, totalUses: 1, notes: '' })
let loadSequence = 0

watch(
  () => [props.show, props.user?.id] as const,
  ([show]) => {
    if (show) {
      resetForm()
      void loadCoupons()
    }
  },
  { immediate: true }
)

function resetForm() {
  Object.assign(form, { minAmount: 100, discountRate: 8, totalUses: 1, notes: '' })
}

async function loadCoupons() {
  if (!props.user) return
  const sequence = ++loadSequence
  loading.value = true
  loadError.value = false
  try {
    const result = await adminAPI.users.listRechargeDiscountCoupons(props.user.id)
    if (sequence === loadSequence) coupons.value = result
  } catch {
    if (sequence === loadSequence) {
      coupons.value = []
      loadError.value = true
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

const isValid = computed(() =>
  Number.isFinite(form.minAmount) && form.minAmount > 0
  && Number.isFinite(form.discountRate) && form.discountRate > 0 && form.discountRate < 10
  && Number.isInteger(form.totalUses) && form.totalUses > 0
)

const couponSummary = computed(() => t('admin.users.rechargeCoupon.summary', {
  amount: Number(form.minAmount || 0).toFixed(2),
  rate: Number(form.discountRate || 0).toString(),
  count: form.totalUses || 0,
}))

function formatAmount(value: number): string {
  return Number(value).toFixed(2)
}

function formatDiscountRate(value: number): string {
  return Number((Number(value) / 10).toFixed(2)).toString()
}

function couponRuleLabel(coupon: RechargeDiscountCoupon): string {
  const params = { amount: formatAmount(coupon.min_recharge_amount), rate: formatDiscountRate(coupon.discount_percent) }
  return t(coupon.min_recharge_amount > 0 ? 'admin.users.rechargeCoupon.couponRule' : 'admin.users.rechargeCoupon.couponRuleNoThreshold', params)
}

function couponState(coupon: RechargeDiscountCoupon): 'active' | 'exhausted' | 'revoked' {
  if (coupon.status === 'revoked') return 'revoked'
  if (coupon.total_uses > 0 && coupon.remaining_uses <= 0) return 'exhausted'
  return 'active'
}

function couponSourceLabel(coupon: RechargeDiscountCoupon): string {
  if (coupon.source_type === 'promo_code') {
    return t('admin.users.rechargeCoupon.sourcePromoCode', { code: coupon.source_code || '-' })
  }
  return t('admin.users.rechargeCoupon.sourceAdmin')
}

function couponStatusLabel(coupon: RechargeDiscountCoupon): string {
  return t(`admin.users.rechargeCoupon.status.${couponState(coupon)}`)
}

function couponStatusClass(coupon: RechargeDiscountCoupon): string {
  return {
    active: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    exhausted: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
    revoked: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  }[couponState(coupon)]
}

async function handleSubmit() {
  if (!props.user || !isValid.value) return
  submitting.value = true
  try {
    await adminAPI.users.issueRechargeDiscountCoupon(props.user.id, {
      min_recharge_amount: form.minAmount,
      discount_rate: form.discountRate,
      total_uses: form.totalUses,
      notes: form.notes.trim(),
    })
    appStore.showSuccess(t('admin.users.rechargeCoupon.success'))
    resetForm()
    await loadCoupons()
    emit('success')
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.rechargeCoupon.failed'))
  } finally {
    submitting.value = false
  }
}
</script>
