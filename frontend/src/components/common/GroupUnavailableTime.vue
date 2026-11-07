<template>
  <span v-if="window" class="text-xs text-amber-700 dark:text-amber-300">
    {{ t('payment.planCard.unavailableTime') }}: {{ window }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { serverTimezoneLabel } from '@/utils/peak-rate'
import { formatUnavailableTimeWindow, type UnavailableTimeFields } from '@/utils/unavailable-time'

const props = defineProps<{ fields?: UnavailableTimeFields | null }>()
const { t } = useI18n()
const appStore = useAppStore()
const window = computed(() => formatUnavailableTimeWindow(
  props.fields,
  serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset),
  t('payment.planCard.nextDay')
))
</script>
