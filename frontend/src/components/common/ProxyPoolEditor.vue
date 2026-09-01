<template>
  <div v-if="proxies.length > 0" class="mt-2 rounded border border-gray-200 p-3 dark:border-dark-600">
    <div class="mb-2 flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
      <span>{{ t('admin.accounts.proxyPool') }}</span>
      <span v-if="modelValue.length" class="font-mono">{{ t('admin.accounts.proxyPoolTotalConcurrency', { count: totalConcurrency }) }}</span>
    </div>
    <div class="space-y-2">
      <label v-for="proxy in proxies" :key="proxy.id" class="flex items-center gap-3 text-sm">
        <input
          type="checkbox"
          :checked="hasProxy(proxy.id)"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="toggleProxy(proxy.id)"
        />
        <span class="min-w-0 flex-1 truncate text-gray-700 dark:text-gray-300">
          {{ proxy.name }} ({{ proxy.host }}:{{ proxy.port }})
        </span>
        <input
          v-if="hasProxy(proxy.id)"
          type="number"
          min="1"
          max="10000"
          class="input w-24"
          :value="concurrencyFor(proxy.id)"
          :aria-label="t('admin.accounts.proxyConcurrency')"
          @input="setConcurrency(proxy.id, ($event.target as HTMLInputElement).value)"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { AccountProxyBindingRequest, Proxy } from '@/types'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  modelValue: AccountProxyBindingRequest[]
  proxies: Proxy[]
}>()

const totalConcurrency = computed(() => props.modelValue.reduce((total, entry) => total + entry.concurrency, 0))

const emit = defineEmits<{
  'update:modelValue': [value: AccountProxyBindingRequest[]]
}>()

const hasProxy = (proxyID: number) => props.modelValue.some((entry) => entry.proxy_id === proxyID)

const concurrencyFor = (proxyID: number) => props.modelValue.find((entry) => entry.proxy_id === proxyID)?.concurrency ?? 1

const toggleProxy = (proxyID: number) => {
  if (hasProxy(proxyID)) {
    emit('update:modelValue', props.modelValue.filter((entry) => entry.proxy_id !== proxyID))
    return
  }
  emit('update:modelValue', [...props.modelValue, { proxy_id: proxyID, concurrency: 1 }])
}

const setConcurrency = (proxyID: number, raw: string) => {
  const concurrency = Math.min(10000, Math.max(1, Number.parseInt(raw, 10) || 1))
  emit('update:modelValue', props.modelValue.map((entry) => entry.proxy_id === proxyID ? { ...entry, concurrency } : entry))
}
</script>
