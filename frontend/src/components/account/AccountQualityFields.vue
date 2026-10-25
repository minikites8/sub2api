<template>
  <fieldset class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
    <legend class="px-1 font-medium">{{ t('gptQos.accountQuality') }}</legend>
    <label class="block"><span class="input-label">{{ t('gptQos.qualityTier') }}</span>
      <select :value="tier" class="input" @change="$emit('update:tier', ($event.target as HTMLSelectElement).value)">
        <option value="unknown">{{ t('gptQos.unknown') }}</option><option value="strong">{{ t('gptQos.strong') }}</option><option value="basic">{{ t('gptQos.basic') }}</option>
      </select>
    </label>
    <label v-if="showPool !== false" class="block"><span class="input-label">{{ t('gptQos.sharedPool') }}</span>
      <input :value="pool" class="input" maxlength="64" pattern="[a-zA-Z0-9_-]*" @input="$emit('update:pool', ($event.target as HTMLInputElement).value)" />
    </label>
    <label v-if="showSource !== false" class="block"><span class="input-label">{{ t('gptQos.upstreamKind') }}</span><select :value="source || 'unknown'" class="input" @change="$emit('update:source', ($event.target as HTMLSelectElement).value)"><option value="unknown">{{ t('gptQos.unknown') }}</option><option value="vm2api">VM2API</option><option value="direct">{{ t('gptQos.direct') }}</option></select></label>
    <p class="input-hint">{{ t('gptQos.accountHint') }}</p>
  </fieldset>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ tier: string; pool: string; source?: string; showPool?: boolean; showSource?: boolean }>()
defineEmits<{ 'update:tier': [value: string]; 'update:pool': [value: string]; 'update:source': [value: string] }>()
const { t } = useI18n()
</script>
