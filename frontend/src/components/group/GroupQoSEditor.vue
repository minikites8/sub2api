<template>
  <fieldset class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
    <legend class="px-1 font-medium">{{ t('gptQos.groupQos') }}</legend>
    <p class="input-hint">{{ t('gptQos.groupHint') }}</p>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div v-if="loaded" class="grid gap-3 sm:grid-cols-2">
      <label><span class="input-label">{{ t('gptQos.tier') }}</span><select v-model="form.qos_tier" class="input" @change="applyTier"><option value="premium">Premium</option><option value="standard">Standard</option><option value="economy">Economy</option></select></label>
      <label v-for="field in fields" :key="field.key"><span class="input-label">{{ t(`gptQos.${field.label}`) }}</span><input v-model.number="form[field.key]" type="number" :min="field.min" :max="field.max" class="input" /></label>
      <label class="flex items-center gap-2"><input v-model="form.allow_basic_fallback" type="checkbox" />{{ t('gptQos.fallback') }}</label>
      <label class="flex items-center gap-2"><input v-model="form.capacity_protection" type="checkbox" />{{ t('gptQos.protection') }}</label>
    </div>
    <button type="button" class="btn btn-secondary" :disabled="!loaded || saving" @click="save">{{ t('gptQos.save') }}</button>
    <span v-if="saved" role="status" class="ml-3 text-green-600">{{ t('gptQos.saved') }}</span>
  </fieldset>
</template>
<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { defaultQoSGroup, getQoSConfig, saveQoSConfig } from '@/api/admin/gptQos'
const props = defineProps<{ groupId: number }>()
const { t } = useI18n()
const form = reactive(defaultQoSGroup())
const loaded = ref(false), saving = ref(false), saved = ref(false), error = ref('')
const fields = [
  { key: 'weight', label: 'weight', min: 1, max: 100 }, { key: 'strong_wait_ms', label: 'strongWait', min: 0, max: 120000 },
  { key: 'basic_wait_ms', label: 'basicWait', min: 0, max: 120000 }, { key: 'max_waiting', label: 'groupMaxWaiting', min: 1, max: 2000 }, { key: 'user_max_waiting', label: 'userMaxWaiting', min: 1, max: 100 },
  { key: 'concurrency', label: 'groupConcurrency', min: 0, max: 100000 }
] as const
const applyTier = () => Object.assign(form, defaultQoSGroup(form.qos_tier))
watch(() => props.groupId, async id => { loaded.value = false; saved.value = false; error.value = ''; try { const c = await getQoSConfig(); Object.assign(form, c.groups[String(id)] || defaultQoSGroup()); loaded.value = true } catch { error.value = t('gptQos.failed') } }, { immediate: true })
async function save() { saving.value = true; error.value = ''; saved.value = false; try { const c = await getQoSConfig(); c.groups[String(props.groupId)] = { ...form }; await saveQoSConfig(c); saved.value = true } catch { error.value = t('gptQos.failed') } finally { saving.value = false } }
</script>
