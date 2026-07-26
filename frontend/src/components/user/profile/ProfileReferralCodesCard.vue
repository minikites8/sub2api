<template>
  <section
    data-testid="profile-referral-codes-panel"
    class="profile-referral-card"
  >
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t('profile.referralCodesTitle') }}
    </h3>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t('profile.referralCodesDescription') }}
    </p>

    <div class="mt-5 grid gap-4 sm:grid-cols-2">
      <div class="profile-referral-subsection">
        <div class="flex items-center justify-between gap-3">
          <span class="text-sm font-medium text-gray-600 dark:text-gray-300">
            {{ t('profile.myAffiliateCode') }}
          </span>
          <span
            v-if="affiliateCode"
            class="profile-referral-code-chip"
          >
            {{ affiliateCode }}
          </span>
          <span v-else class="text-sm text-gray-400 dark:text-gray-500">
            {{ t('common.none') }}
          </span>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          {{
            inviterBound
              ? t('profile.affiliateInviterBound')
              : t('profile.affiliateInviterEmpty')
          }}
        </p>
        <div
          v-if="inviterAffiliateCode"
          class="profile-referral-list-row mt-3 text-sm"
        >
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('profile.usedAffiliateCode') }}
          </span>
          <span class="font-mono font-semibold text-gray-800 dark:text-gray-100">
            {{ inviterAffiliateCode }}
          </span>
        </div>
      </div>

      <div class="profile-referral-subsection">
        <div class="mb-3 flex items-center justify-between gap-3">
          <span class="text-sm font-medium text-gray-600 dark:text-gray-300">
            {{ t('profile.usedPromoCodes') }}
          </span>
          <span class="text-xs text-gray-400 dark:text-gray-500">
            {{ usedPromoCodes.length }}
          </span>
        </div>

        <div v-if="usedPromoCodes.length" class="space-y-2">
          <div
            v-for="usage in usedPromoCodes"
            :key="`${usage.code}-${usage.used_at}`"
            class="profile-referral-list-row"
          >
            <span class="font-mono font-semibold text-gray-800 dark:text-gray-100">
              {{ usage.code }}
            </span>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ formatPromoUsageLabel(usage) }}
            </span>
          </div>
        </div>
        <p v-else class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.noUsedPromoCodes') }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { User, UserPromoCodeUsage } from '@/types'

const props = defineProps<{
  user: User | null
}>()

const { t } = useI18n()

const usedPromoCodes = computed(() => props.user?.used_promo_codes ?? [])
const affiliateCode = computed(() => props.user?.affiliate?.aff_code?.trim() || '')
const inviterAffiliateCode = computed(() => props.user?.affiliate?.inviter_aff_code?.trim() || '')
const inviterBound = computed(() => Boolean(props.user?.affiliate?.inviter_id))

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function formatPromoUsageLabel(usage: UserPromoCodeUsage): string {
  const bonus = Number(usage.bonus_amount || 0)
  if (bonus > 0) {
    return t('profile.promoBonusAmount', { amount: formatCurrency(bonus) })
  }

  const rawDate = usage.used_at?.trim()
  if (!rawDate) {
    return t('profile.promoUsed')
  }
  const date = new Date(rawDate)
  if (Number.isNaN(date.getTime())) {
    return t('profile.promoUsed')
  }
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  }).format(date)
}
</script>

<style scoped>
.profile-referral-card {
  border: 1px solid var(--md-outline-variant);
  border-radius: 12px;
  background: var(--md-surface);
  color: var(--md-on-surface);
  box-shadow: none;
  padding: 20px;
  transition: border-color 0.2s ease;
}

.profile-referral-card:hover {
  border-color: var(--md-primary);
}

.profile-referral-subsection,
.profile-referral-list-row {
  border: 1px solid var(--md-outline-variant);
  border-radius: 10px;
  background: var(--md-surface-container-low);
  box-shadow: none;
}

.profile-referral-subsection {
  padding: 1rem;
}

.profile-referral-list-row {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.625rem 0.75rem;
}

.profile-referral-code-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid var(--md-outline-variant);
  border-radius: 999px;
  background: var(--md-surface-container-low);
  padding: 0.25rem 0.75rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--md-on-surface);
}
</style>
