<template>
  <div class="space-y-2 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="group-gpt-auto">
    <div class="flex items-center justify-between gap-3"><div><p class="text-sm font-medium">{{ t('gptAuto.group.allow') }}</p><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.group.help') }}</p></div><Toggle :model-value="modelValue" @update:model-value="setEnabled" data-testid="group-gpt-auto-toggle" /></div>
    <p class="text-xs">{{ t(modelValue ? 'gptAuto.group.enabled' : 'gptAuto.group.disabled') }}</p>
    <p v-if="blocked" role="alert" class="text-xs text-amber-700 dark:text-amber-300">{{ t('gptAuto.group.allowlistBlocked') }} <button type="button" class="underline" @click="permit">{{ t('gptAuto.group.addPermission') }}</button></p>
    <p v-if="state && !state.live_enabled" class="text-xs text-amber-700 dark:text-amber-300">{{ t('gptAuto.group.globalOff') }}</p>
    <p v-if="state?.internal_only" class="text-xs text-amber-700 dark:text-amber-300">{{ t('gptAuto.group.internalOnly') }}</p>
    <p v-if="!state" class="text-xs text-gray-500">{{ t('gptAuto.group.unknown') }}</p>
    <RouterLink to="/admin/gpt-auto?tab=pricing" class="text-xs text-primary-600 underline">{{ t('gptAuto.group.pricing') }}</RouterLink>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { getPricing, type GPTAutoPricingState } from '@/api/admin/gptAuto'
import { buildModelAllowlistConfig, type ModelAllowlistState } from '@/views/admin/groupModelAllowlist'
const props = defineProps<{ modelValue: boolean; allowlist: ModelAllowlistState }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; 'update:allowlist': [ModelAllowlistState] }>()
const { t } = useI18n()
const state = ref<GPTAutoPricingState | null>(null)
const blocked = computed(() => {
  const list = buildModelAllowlistConfig(props.allowlist)
  return props.modelValue && list.enabled && !list.models.some(m => new RegExp('^' + m.replace(/[.+?^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*') + '$', 'i').test('gpt-auto'))
})
function permit() {
  const next: ModelAllowlistState = { ...props.allowlist, savedModels: [...props.allowlist.savedModels], items: props.allowlist.items.map(item => ({ ...item })) }
  if (!next.items.length) next.items = next.savedModels.map(id => ({ id, selected: true }))
  const item = next.items.find(i => i.id.toLowerCase() === 'gpt-auto')
  if (item) item.selected = true
  else next.items.push({ id: 'gpt-auto', selected: true })
  if (!next.savedModels.includes('gpt-auto')) next.savedModels.push('gpt-auto')
  emit('update:allowlist', next)
}
function setEnabled(value: boolean) { if (value) permit(); emit('update:modelValue', value) }
onMounted(async () => { try { state.value = await getPricing() } catch { state.value = null } })
</script>
