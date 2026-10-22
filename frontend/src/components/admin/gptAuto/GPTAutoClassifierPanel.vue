<template>
  <div class="space-y-6" data-testid="gpt-auto-classifier">
    <div v-if="loading" class="flex justify-center py-12">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>
    <p v-else-if="loadError" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ loadError }}</p>

    <template v-else-if="view && draft">
      <div class="rounded-lg border border-primary-200 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-800/60 dark:bg-primary-900/20 dark:text-primary-200">
        {{ t('gptAuto.classifier.notice') }}
      </div>

      <section class="card">
        <header class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.classifier.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.classifier.hint') }}</p>
          </div>
          <div class="text-right text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-auto-classifier-version">
            <p>{{ t('gptAuto.classifier.revision', { revision: view.instructions.revision ?? 0 }) }} · <span class="font-mono">{{ view.hash.slice(0, 12) }}</span></p>
            <p v-if="view.instructions.updated_at">{{ t('gptAuto.classifier.updated', { time: formatDateTime(view.instructions.updated_at), user: view.instructions.updated_by ?? '—' }) }}</p>
            <span v-if="view.is_default" class="mt-1 inline-block rounded-full bg-gray-100 px-2 py-0.5 dark:bg-dark-700">{{ t('gptAuto.classifier.usingDefaults') }}</span>
          </div>
        </header>
        <div class="space-y-5 p-6">
          <div v-for="field in fields" :key="field">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="input-label">{{ t(`gptAuto.classifier.fields.${field}`) }}</span>
              <button type="button" class="btn btn-ghost btn-sm" :disabled="draft[field] === view.defaults[field]" :data-testid="`gpt-auto-instr-default-${field}`" @click="draft[field] = view.defaults[field]">
                {{ t('gptAuto.classifier.restoreField') }}
              </button>
            </div>
            <p class="input-hint">{{ t(`gptAuto.classifier.fieldHints.${field}`) }}</p>
            <textarea v-model="draft[field]" rows="6" :maxlength="view.limits.max_runes" class="input mt-1 text-sm" :class="fieldErrors[field] ? 'input-error' : ''" :data-testid="`gpt-auto-instr-${field}`"></textarea>
            <p class="mt-1 text-xs" :class="fieldErrors[field] ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'">
              {{ t('gptAuto.classifier.counter', { count: runeCount(draft[field]), max: view.limits.max_runes, min: view.limits.min_runes }) }}
            </p>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.classifier.descriptionsNote') }}</p>
        </div>
        <div class="flex flex-wrap items-center justify-end gap-3 border-t border-gray-100 px-6 py-3 dark:border-dark-700">
          <p v-if="hasErrors" class="mr-auto text-sm text-red-600 dark:text-red-400">{{ t('gptAuto.config.fixErrors') }}</p>
          <p v-else-if="dirty" class="mr-auto text-sm text-amber-600 dark:text-amber-400">{{ t('gptAuto.classifier.unsaved') }}</p>
          <button type="button" class="btn btn-secondary" data-testid="gpt-auto-instr-restore-all" @click="restoreAll">{{ t('gptAuto.classifier.restoreAll') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="!dirty" data-testid="gpt-auto-instr-discard" @click="discard">{{ t('gptAuto.config.reset') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="previewing || hasErrors" data-testid="gpt-auto-instr-preview" @click="runPreview">{{ previewing ? t('gptAuto.common.working') : t('gptAuto.classifier.preview') }}</button>
          <button type="button" class="btn btn-primary" :disabled="saving || !dirty || hasErrors" data-testid="gpt-auto-instr-save" @click="save">{{ saving ? t('common.saving') : t('gptAuto.config.save') }}</button>
        </div>
      </section>

      <section v-if="preview" class="card p-6" data-testid="gpt-auto-instr-preview-result">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.classifier.previewTitle') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('gptAuto.classifier.previewHint', { hash: preview.hash.slice(0, 12) }) }}
          <span v-if="preview.same_as_saved"> · {{ t('gptAuto.classifier.previewSame') }}</span>
        </p>
        <div class="mt-3 space-y-3">
          <div v-for="(question, id) in preview.effective.questions" :key="id" class="rounded border border-gray-100 p-3 text-xs dark:border-dark-700">
            <p class="font-mono font-semibold text-gray-800 dark:text-gray-200">{{ id }} <span class="font-normal text-gray-500">({{ question.type }})</span></p>
            <p class="mt-1 whitespace-pre-wrap text-gray-700 dark:text-gray-300">{{ question.instructions }}</p>
            <ul class="mt-2 space-y-0.5 text-gray-500 dark:text-gray-400">
              <li v-for="(criterion, key) in question.criteria" :key="key"><span class="font-mono">{{ key }}</span>: {{ criterion }}</li>
            </ul>
          </div>
        </div>
      </section>

      <section class="card overflow-hidden" data-testid="gpt-auto-instr-history">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.classifier.historyTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.classifier.historyHint') }}</p>
        </header>
        <div v-if="view.history.length" class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{{ t('gptAuto.classifier.historyColumns.created') }}</th>
                <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{{ t('gptAuto.classifier.historyColumns.revision') }}</th>
                <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{{ t('gptAuto.classifier.historyColumns.hash') }}</th>
                <th class="px-4 py-2 text-left text-xs font-medium text-gray-500">{{ t('gptAuto.classifier.historyColumns.author') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="record in view.history" :key="record.hash" :class="record.hash === view.hash ? 'font-semibold' : ''">
                <td class="px-4 py-2 text-gray-600 dark:text-gray-300">{{ formatDateTime(record.created_at) }}</td>
                <td class="px-4 py-2 tabular-nums">{{ record.revision }}</td>
                <td class="px-4 py-2 font-mono text-xs">{{ record.hash.slice(0, 12) }}<span v-if="record.hash === view.hash" class="ml-2 text-emerald-600">{{ t('gptAuto.classifier.current') }}</span></td>
                <td class="px-4 py-2 text-gray-500">{{ record.created_by ?? t('gptAuto.classifier.recordedByShadow') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.common.noData') }}</p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { gptAutoAPI, gptAutoInstructionFields, type GPTAutoClassifierInstructions, type GPTAutoClassifierPreview, type GPTAutoClassifierView, type GPTAutoInstructionField } from '@/api/admin/gptAuto'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const fields = gptAutoInstructionFields

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const previewing = ref(false)
const view = ref<GPTAutoClassifierView | null>(null)
const draft = ref<Record<GPTAutoInstructionField, string> | null>(null)
const preview = ref<GPTAutoClassifierPreview | null>(null)

const runeCount = (text: string) => [...text.trim()].length

function textOf(instructions: GPTAutoClassifierInstructions): Record<GPTAutoInstructionField, string> {
  return Object.fromEntries(fields.map((field) => [field, instructions[field] ?? ''])) as Record<GPTAutoInstructionField, string>
}

const fieldErrors = computed(() => {
  const errors: Partial<Record<GPTAutoInstructionField, boolean>> = {}
  if (!draft.value || !view.value) return errors
  for (const field of fields) {
    const count = runeCount(draft.value[field])
    errors[field] = count < view.value.limits.min_runes || count > view.value.limits.max_runes
  }
  return errors
})
const hasErrors = computed(() => Object.values(fieldErrors.value).some(Boolean))
const dirty = computed(() => !!view.value && !!draft.value && fields.some((field) => draft.value![field].trim() !== (view.value!.instructions[field] ?? '').trim()))

function apply(next: GPTAutoClassifierView) {
  view.value = next
  draft.value = textOf(next.instructions)
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    apply(await gptAutoAPI.getClassifier())
  } catch (err) {
    loadError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function restoreAll() {
  if (view.value) draft.value = textOf(view.value.defaults)
}

function discard() {
  if (view.value) draft.value = textOf(view.value.instructions)
  preview.value = null
}

async function runPreview() {
  if (!draft.value) return
  previewing.value = true
  try {
    preview.value = await gptAutoAPI.previewClassifier({ ...draft.value })
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('gptAuto.classifier.previewFailed')))
  } finally {
    previewing.value = false
  }
}

async function save() {
  if (!draft.value || hasErrors.value) return
  saving.value = true
  try {
    apply(await gptAutoAPI.updateClassifier({ ...draft.value }))
    preview.value = null
    appStore.showSuccess(t('gptAuto.classifier.saved'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('gptAuto.config.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => { void load() })

defineExpose({ load, dirty })
</script>
