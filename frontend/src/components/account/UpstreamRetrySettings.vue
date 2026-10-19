<script setup lang="ts">
import { useId } from 'vue'
import { useI18n } from 'vue-i18n'
import { MAX_UPSTREAM_RETRIES, type UpstreamMaxRetries } from './upstreamRetry'

defineProps<{
  enabled: boolean
  maxRetries: UpstreamMaxRetries
}>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:maxRetries': [value: UpstreamMaxRetries]
}>()
const { t } = useI18n()
const id = useId()

function updateMaxRetries(event: Event) {
  const input = event.target as HTMLInputElement
  emit('update:maxRetries', input.value === '' ? '' : input.valueAsNumber)
}
</script>

<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="upstream-retry-settings">
    <label class="flex cursor-pointer items-center justify-between gap-4" :for="`${id}-enabled`">
      <span class="input-label mb-0">{{ t('admin.accounts.upstreamRetry.title') }}</span>
      <input
        :id="`${id}-enabled`"
        type="checkbox"
        :checked="enabled"
        :aria-describedby="`${id}-hint`"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        data-testid="upstream-retry-enabled"
        @change="emit('update:enabled', ($event.target as HTMLInputElement).checked)"
      />
    </label>
    <p :id="`${id}-hint`" class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.upstreamRetry.hint') }}
    </p>
    <div v-if="enabled" class="space-y-2">
      <label :for="`${id}-limit`" class="input-label">{{ t('admin.accounts.upstreamRetry.maxRetries') }}</label>
      <input
        :id="`${id}-limit`"
        type="number"
        min="0"
        :max="MAX_UPSTREAM_RETRIES"
        step="1"
        class="input"
        :value="maxRetries"
        :placeholder="t('admin.accounts.upstreamRetry.defaultLimit')"
        :aria-describedby="`${id}-limit-hint`"
        data-testid="upstream-retry-max-retries"
        @input="updateMaxRetries"
      />
      <p :id="`${id}-limit-hint`" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.upstreamRetry.limitHint') }}
      </p>
    </div>
  </section>
</template>
