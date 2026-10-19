<template>
  <div class="space-y-2">
    <ol class="space-y-2" :aria-label="t('keys.groupOrder')">
      <li v-for="(id, index) in modelValue" :key="id" class="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600" data-test="bound-group">
        <span class="text-sm tabular-nums text-gray-500">{{ index + 1 }}.</span>
        <div class="min-w-0 flex-1">
          <slot name="group" :option="options.find(option => option.value === id)" :id="id">
            {{ options.find(option => option.value === id)?.label ?? `#${id}` }}
          </slot>
        </div>
        <button type="button" class="btn btn-secondary px-2 py-1" :disabled="disabled || index === 0" :aria-label="t('keys.moveGroupUp')" @click="move(index, -1)">↑</button>
        <button type="button" class="btn btn-secondary px-2 py-1" :disabled="disabled || index === modelValue.length - 1" :aria-label="t('keys.moveGroupDown')" @click="move(index, 1)">↓</button>
        <button type="button" class="btn btn-secondary px-2 py-1" :disabled="disabled" :aria-label="t('keys.removeGroup')" @click="emit('update:modelValue', modelValue.filter(value => value !== id))">×</button>
      </li>
    </ol>
    <Select
      :id="id"
      :model-value="null"
      :options="options.filter(option => !modelValue.includes(option.value))"
      :placeholder="t('keys.addGroup')"
      :empty-text="t('common.noGroupsAvailable')"
      :search-placeholder="t('keys.searchGroup')"
      :aria-label="t('keys.addGroup')"
      :disabled="disabled || modelValue.length >= 32"
      searchable
      @update:model-value="add"
    >
      <template v-if="$slots.option" #option="scope"><slot name="option" v-bind="scope" /></template>
    </Select>
    <p class="input-hint">{{ t('keys.orderedGroupsHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'

const props = defineProps<{
  modelValue: number[]
  options: Array<{ value: number; label: string }>
  id?: string
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: number[]] }>()
const { t } = useI18n()

function add(value: string | number | boolean | null) {
  if (typeof value !== 'number' || props.disabled || props.modelValue.length >= 32 || props.modelValue.includes(value)) return
  emit('update:modelValue', [...props.modelValue, value])
}

function move(index: number, offset: number) {
  const target = index + offset
  if (props.disabled || target < 0 || target >= props.modelValue.length) return
  const ids = [...props.modelValue]
  ;[ids[index], ids[target]] = [ids[target], ids[index]]
  emit('update:modelValue', ids)
}
</script>
