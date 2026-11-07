<template>
  <div class="border-t pt-4">
    <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
      <input v-model="enabled" type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
      <span>{{ t('admin.groups.unavailableTime.enable') }}</span>
    </label>
    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.groups.unavailableTime.hint', { timezone: timezoneLabel }) }}
    </p>
    <div v-if="enabled" class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
      <label>
        <span class="input-label">{{ t('admin.groups.unavailableTime.start') }}</span>
        <input v-model="start" type="time" required class="input" />
      </label>
      <label>
        <span class="input-label">{{ t('admin.groups.unavailableTime.end') }}</span>
        <input v-model="end" type="time" required class="input" />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { serverTimezoneLabel } from '@/utils/peak-rate'

const enabled = defineModel<boolean>('enabled', { required: true })
const start = defineModel<string>('start', { required: true })
const end = defineModel<string>('end', { required: true })
const { t } = useI18n()
const appStore = useAppStore()
const timezoneLabel = computed(() => serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
</script>
