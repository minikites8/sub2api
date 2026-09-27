<template>
  <div class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4">
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
      {{ t('admin.groups.routingPolicy.title') }}
    </h4>
    <div class="flex items-center justify-between gap-4">
      <label :for="idPrefix + '-enable-bps'" class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.groups.routingPolicy.enableBPS') }}
      </label>
      <Toggle
        :id="idPrefix + '-enable-bps'"
        :data-testid="idPrefix + '-enable-bps'"
        :aria-label="t('admin.groups.routingPolicy.enableBPS')"
        :model-value="enableBps"
        @update:model-value="$emit('update:enableBps', $event)"
      />
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
      {{ t('admin.groups.routingPolicy.bpsHint') }}
    </p>
    <div class="mt-4">
      <p :id="idPrefix + '-strategy-label'" class="text-sm text-gray-600 dark:text-gray-400 mb-2">
        {{ t('admin.groups.routingPolicy.strategy') }}
      </p>
      <div class="grid grid-cols-3 gap-1 rounded-lg bg-gray-100 dark:bg-dark-700 p-1"
        role="group" :aria-labelledby="idPrefix + '-strategy-label'">
        <button v-for="option in strategies" :key="option.value" type="button"
          class="rounded-md px-2 py-2 text-sm transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
          :class="schedulingStrategy === option.value
            ? 'bg-white dark:bg-dark-500 text-primary-600 dark:text-primary-400 shadow-sm'
            : 'text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-dark-600'"
          :aria-pressed="schedulingStrategy === option.value"
          :data-testid="idPrefix + '-strategy-' + option.value"
          @click="$emit('update:schedulingStrategy', option.value)">
          {{ t('admin.groups.routingPolicy.' + option.label) }}
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mt-2">
        {{ t('admin.groups.routingPolicy.strategyHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import type { GroupSchedulingStrategy } from '@/types'

defineProps<{ idPrefix: string; enableBps: boolean; schedulingStrategy: GroupSchedulingStrategy }>()
defineEmits<{
  (e: 'update:enableBps', value: boolean): void
  (e: 'update:schedulingStrategy', value: GroupSchedulingStrategy): void
}>()
const { t } = useI18n()
const strategies: { value: GroupSchedulingStrategy; label: string }[] = [
  { value: 'balanced', label: 'balanced' },
  { value: 'priority_5h', label: 'priority5h' },
  { value: 'priority_weekly', label: 'priorityWeekly' }
]
</script>
