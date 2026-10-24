<template>
  <BaseDialog :show="show" :title="t('gptAuto.export.title')" width="wide" @close="$emit('close')">
    <div class="space-y-4 text-sm" data-testid="gpt-auto-export-dialog">
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.export.hint') }}</p>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
        <label>
          <span class="input-label">{{ t('gptAuto.export.format') }}</span>
          <Select v-model="form.format" data-testid="gpt-auto-export-format" :options="[{ value: 'csv', label: 'CSV' }, { value: 'jsonl', label: 'JSONL' }]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.start') }}</span>
          <input v-model="form.start" type="datetime-local" class="input" data-testid="gpt-auto-export-start" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.end') }}</span>
          <input v-model="form.end" type="datetime-local" class="input" data-testid="gpt-auto-export-end" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.mode') }}</span>
          <Select v-model="form.mode" data-testid="gpt-auto-export-mode" :options="[{ value: '', label: t('gptAuto.export.modeAll') }, { value: 'shadow', label: t('gptAuto.live.modes.shadow') }, { value: 'live', label: t('gptAuto.live.modes.live') }]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.groupId') }}</span>
          <input v-model="form.group_id" type="number" min="1" class="input" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.requestedModel') }}</span>
          <input v-model.trim="form.requested_model" type="text" maxlength="128" class="input" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.jevModel') }}</span>
          <Select v-model="form.jev_model" :options="[{ value: '', label: t('gptAuto.common.all') }, ...(historicalModels).map((model) => ({ value: model, label: model }))]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.finalModel') }}</span>
          <Select v-model="form.final_model" :options="[{ value: '', label: t('gptAuto.common.all') }, ...(historicalModels).map((model) => ({ value: model, label: model }))]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.effort') }}</span>
          <Select v-model="form.final_effort" :options="[{ value: '', label: t('gptAuto.common.all') }, ...(efforts).map((effort) => ({ value: effort, label: labels.effort(effort) }))]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.category') }}</span>
          <Select v-model="form.task_category" :options="[{ value: '', label: t('gptAuto.common.all') }, ...(categories).map((category) => ({ value: category, label: labels.category(category) }))]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.requestStatus') }}</span>
          <Select v-model="form.request_status" :options="[{ value: '', label: t('gptAuto.common.all') }, ...(requestStatuses).map((status) => ({ value: status, label: t(`gptAuto.live.requestStatuses.${status}`) }))]" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.status') }}</span>
          <Select v-model="form.evaluation_status" :options="[{ value: '', label: t('gptAuto.common.all') }, { value: 'success', label: labels.evaluationStatus('success') }, { value: 'failed', label: labels.evaluationStatus('failed') }, { value: 'skipped', label: labels.evaluationStatus('skipped') }]" />
        </label>
        <div class="grid grid-cols-2 gap-2">
          <label><span class="input-label">{{ t('gptAuto.export.complexityMin') }}</span><input v-model="form.complexity_min" type="number" step="0.1" min="0" max="4" class="input" /></label>
          <label><span class="input-label">{{ t('gptAuto.export.complexityMax') }}</span><input v-model="form.complexity_max" type="number" step="0.1" min="0" max="4" class="input" /></label>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <label><span class="input-label">{{ t('gptAuto.export.confidenceMin') }}</span><input v-model="form.confidence_min" type="number" step="0.05" min="0" max="1" class="input" /></label>
          <label><span class="input-label">{{ t('gptAuto.export.confidenceMax') }}</span><input v-model="form.confidence_max" type="number" step="0.05" min="0" max="1" class="input" /></label>
        </div>
        <label>
          <span class="input-label">{{ t('gptAuto.export.upstreamAccount') }}</span>
          <input v-model="form.upstream_account_id" type="number" min="1" class="input" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.classifierSchema') }}</span>
          <input v-model.trim="form.classifier_schema" type="text" maxlength="64" class="input" placeholder="jev-questions-v2" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.stateVersion') }}</span>
          <input v-model.trim="form.state_version" type="text" maxlength="64" class="input" placeholder="state-v3" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.export.policyVersion') }}</span>
          <input v-model.trim="form.policy_version" type="text" maxlength="64" class="input" />
        </label>
        <label class="flex items-center gap-2 self-end pb-2 text-gray-700 dark:text-gray-300">
          <input v-model="form.fallback_only" type="checkbox" class="rounded border-gray-300" />
          {{ t('gptAuto.export.fallbackOnly') }}
        </label>
      </div>

      <div v-if="summary" class="rounded-lg border p-3 text-xs" :class="summary.truncated ? 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700/60 dark:bg-amber-900/20 dark:text-amber-200' : 'border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'" data-testid="gpt-auto-export-summary">
        {{ summary.truncated ? t('gptAuto.export.truncated', { rows: summary.rows, max: summary.max_rows }) : t('gptAuto.export.rows', { rows: summary.rows }) }}
      </div>
      <p v-if="status" class="text-xs text-gray-600 dark:text-gray-300" data-testid="gpt-auto-export-status">{{ status }}</p>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="gpt-auto-export-error">{{ error }}</p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="busy" data-testid="gpt-auto-export-check" @click="check">{{ t('gptAuto.export.check') }}</button>
        <button type="button" class="btn btn-primary" :disabled="busy" data-testid="gpt-auto-export-download" @click="download">
          {{ busy ? t('gptAuto.export.downloading') : t('gptAuto.export.download') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import Select from '@/components/common/Select.vue'
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { gptAutoAPI, type GPTAutoExportParams, type GPTAutoExportPreset, type GPTAutoExportSummary, type GPTAutoMode } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatBytes, saveBlob } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const props = defineProps<{ show: boolean; preset: GPTAutoExportPreset }>()
defineEmits<{ close: [] }>()

const { t } = useI18n()
const labels = useGPTAutoLabels()

// The retired model stays selectable: historical decisions remain exportable.
const historicalModels = ['gpt-5.6-luna', 'gpt-6-luna', 'gpt-6.1-sol', 'gpt-5.6-sol']
const efforts = ['none', 'low', 'medium', 'high', 'xhigh', 'max']
const categories = ['general_conversation', 'writing_transformation', 'coding_debugging', 'mathematical_reasoning', 'research_analysis', 'agent_tool_workflow']
const requestStatuses = ['succeeded', 'failed', 'rejected', 'client_cancelled'] as const

function toLocalInput(date: Date): string {
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

const blank = () => ({
  format: 'csv' as 'csv' | 'jsonl',
  start: '',
  end: '',
  mode: '' as GPTAutoMode | '',
  group_id: '' as string | number,
  requested_model: '',
  jev_model: '',
  final_model: '',
  final_effort: '',
  task_category: '',
  request_status: '' as GPTAutoExportParams['request_status'],
  evaluation_status: '' as GPTAutoExportParams['evaluation_status'],
  complexity_min: '' as string | number,
  complexity_max: '' as string | number,
  confidence_min: '' as string | number,
  confidence_max: '' as string | number,
  upstream_account_id: '' as string | number,
  classifier_schema: '',
  state_version: '',
  policy_version: '',
  fallback_only: false,
})
const form = reactive(blank())
const summary = ref<GPTAutoExportSummary | null>(null)
const busy = ref(false)
const error = ref('')
const status = ref('')

// Each opening starts from the log's current filters.
watch(() => props.show, (open) => {
  if (!open) return
  Object.assign(form, blank(), {
    start: toLocalInput(props.preset.start),
    end: toLocalInput(props.preset.end),
    mode: props.preset.mode ?? '',
    final_model: props.preset.final_model ?? '',
    final_effort: props.preset.final_effort ?? '',
    evaluation_status: props.preset.evaluation_status ?? '',
    group_id: props.preset.group_id ?? '',
  })
  summary.value = null
  error.value = ''
  status.value = ''
}, { immediate: true })

function optionalNumber(value: string | number): number | undefined {
  if (value === '' || value === null || value === undefined) return undefined
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function params(): GPTAutoExportParams | null {
  const start = new Date(form.start)
  const end = new Date(form.end)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || start >= end) {
    error.value = t('gptAuto.export.invalidRange')
    return null
  }
  const groupId = optionalNumber(form.group_id)
  const accountId = optionalNumber(form.upstream_account_id)
  return {
    format: form.format,
    start_time: start.toISOString(),
    end_time: end.toISOString(),
    mode: form.mode,
    group_id: groupId && groupId > 0 ? Math.floor(groupId) : undefined,
    requested_model: form.requested_model,
    jev_model: form.jev_model,
    final_model: form.final_model,
    final_effort: form.final_effort,
    task_category: form.task_category,
    request_status: form.request_status,
    evaluation_status: form.evaluation_status,
    complexity_min: optionalNumber(form.complexity_min),
    complexity_max: optionalNumber(form.complexity_max),
    confidence_min: optionalNumber(form.confidence_min),
    confidence_max: optionalNumber(form.confidence_max),
    upstream_account_id: accountId && accountId > 0 ? Math.floor(accountId) : undefined,
    classifier_schema: form.classifier_schema,
    state_version: form.state_version,
    policy_version: form.policy_version,
    fallback_only: form.fallback_only,
  }
}

async function check(): Promise<boolean> {
  error.value = ''
  const query = params()
  if (!query) return false
  busy.value = true
  try {
    const { format: _format, ...countParams } = query
    void _format
    summary.value = await gptAutoAPI.countExport(countParams)
    return true
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAuto.export.failed'))
    return false
  } finally {
    busy.value = false
  }
}

async function download() {
  // The count runs the same server-side validation and returns readable
  // errors; a streamed file cannot carry a JSON error body.
  if (!(await check())) return
  const query = params()
  if (!query) return
  busy.value = true
  status.value = t('gptAuto.export.streaming')
  try {
    const file = await gptAutoAPI.exportDecisions(query)
    saveBlob(file.blob, file.filename)
    status.value = t('gptAuto.export.done', { file: file.filename, size: formatBytes(file.blob.size) })
  } catch (err) {
    status.value = ''
    error.value = extractApiErrorMessage(err, t('gptAuto.export.failed'))
  } finally {
    busy.value = false
  }
}
</script>
