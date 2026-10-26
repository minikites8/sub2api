<template>
  <fieldset :disabled="disabled || saving" class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
    <legend class="px-1 font-medium">{{ t('gptQos.groupQos') }}</legend>
    <p class="input-hint">{{ t(embedded ? 'gptQos.groupHint' : 'gptQos.groupStandaloneHint') }}</p>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div v-if="loaded" class="grid gap-3 sm:grid-cols-2">
      <label><span class="input-label">{{ t('gptQos.tier') }}</span><select v-model="form.qos_tier" class="input" @change="applyTier"><option value="premium">Premium</option><option value="standard">Standard</option><option value="economy">Economy</option></select></label>
      <label v-for="field in fields" :key="field.key"><span class="input-label">{{ t(`gptQos.${field.label}`) }}</span><input v-model.number="form[field.key]" type="number" :min="field.min" :max="field.max" class="input" /></label>
      <label class="flex items-center gap-2"><input v-model="form.allow_basic_fallback" type="checkbox" />{{ t('gptQos.fallback') }}</label>
      <label class="flex items-center gap-2"><input v-model="form.capacity_protection" type="checkbox" />{{ t('gptQos.protection') }}</label>
    </div>
    <button v-if="!embedded" type="button" class="btn btn-secondary" :disabled="!loaded || saving" @click="saveFromButton">{{ t('gptQos.save') }}</button>
    <span v-if="saved" role="status" class="ml-3 text-green-600">{{ t('gptQos.saved') }}</span>
  </fieldset>
</template>
<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { defaultQoSGroup, getQoSConfig, saveQoSConfig } from '@/api/admin/gptQos'
const props = defineProps<{ groupId: number; embedded?: boolean; disabled?: boolean }>()
const { t } = useI18n()
const form = reactive(defaultQoSGroup())
const baseline = ref(JSON.stringify(form))
const dirty = computed(() => JSON.stringify(form) !== baseline.value)
const loaded = ref(false), saving = ref(false), saved = ref(false), error = ref('')
const fields = [
  { key: 'weight', label: 'weight', min: 1, max: 100 }, { key: 'strong_wait_ms', label: 'strongWait', min: 0, max: 120000 },
  { key: 'basic_wait_ms', label: 'basicWait', min: 0, max: 120000 }, { key: 'max_waiting', label: 'groupMaxWaiting', min: 1, max: 2000 }, { key: 'user_max_waiting', label: 'userMaxWaiting', min: 1, max: 100 },
  { key: 'concurrency', label: 'groupConcurrency', min: 0, max: 100000 }
] as const
const applyTier = () => Object.assign(form, defaultQoSGroup(form.qos_tier))
watch(() => props.groupId, async (id, _previous, onCleanup) => {
  let active = true
  onCleanup(() => { active = false })
  loaded.value = false
  saved.value = false
  error.value = ''
  Object.assign(form, defaultQoSGroup())
  baseline.value = JSON.stringify(form)
  try {
    const c = await getQoSConfig()
    if (!active) return
    const stored = c.groups[String(id)]
    Object.assign(form, defaultQoSGroup(stored?.qos_tier), stored)
    baseline.value = JSON.stringify(form)
    loaded.value = true
  } catch (e: unknown) {
    if (active) error.value = errorMessage(e)
  }
}, { immediate: true })

function errorMessage(e: unknown): string {
  return e && typeof e === 'object' && 'message' in e && typeof e.message === 'string'
    ? e.message : t('gptQos.failed')
}

async function save() {
  // An unrelated group edit must not rewrite the scheduling policy, especially
  // when this optional editor has not loaded or its endpoint is unavailable.
  if (!dirty.value) return
  if (!loaded.value || saving.value) throw new Error(t('gptQos.failed'))
  const id = props.groupId
  const next = { ...form }
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    const c = await getQoSConfig()
    const result = await saveQoSConfig({ ...c, groups: { ...c.groups, [String(id)]: next } })
    const persisted = result.groups[String(id)]
    if (!persisted || Object.keys(next).some(key => persisted[key as keyof typeof next] !== next[key as keyof typeof next])) {
      throw new Error(t('gptQos.saveNotConfirmed'))
    }
    if (props.groupId === id) {
      baseline.value = JSON.stringify(next)
      saved.value = true
    }
  } catch (e: unknown) {
    if (props.groupId === id) error.value = errorMessage(e)
    throw e
  } finally {
    saving.value = false
  }
}
async function saveFromButton() {
  try { await save() } catch { /* The editor shows the persistence error. */ }
}
defineExpose({ save })
</script>
