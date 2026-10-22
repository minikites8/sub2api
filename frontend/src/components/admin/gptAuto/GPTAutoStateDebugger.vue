<template>
  <section class="card space-y-4 p-6" data-testid="gpt-auto-debugger">
    <div>
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.debugger.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.hint') }}</p>
    </div>
    <p class="rounded bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t('gptAuto.debugger.synthetic') }}</p>

    <div class="flex flex-wrap gap-2">
      <button v-for="example in examples" :key="example" type="button" class="btn btn-secondary btn-sm" :data-testid="`gpt-auto-debug-example-${example}`" @click="loadExample(example)">
        {{ t(`gptAuto.debugger.examples.${example}`) }}
      </button>
    </div>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-5">
      <label>
        <span class="input-label">{{ t('gptAuto.lab.endpoint') }}</span>
        <select v-model="endpoint" class="input" data-testid="gpt-auto-debug-endpoint">
          <option value="chat_completions">{{ labels.endpoint('chat_completions') }}</option>
          <option value="responses">{{ labels.endpoint('responses') }}</option>
        </select>
      </label>
      <label>
        <span class="input-label">{{ t('gptAuto.config.classifier.contextStrategy') }}</span>
        <select v-model="strategy" class="input">
          <option value="">{{ t('gptAuto.debugger.saved') }}</option>
          <option value="task_aware">{{ t('gptAuto.config.classifier.strategies.task_aware') }}</option>
          <option value="recent_turns">{{ t('gptAuto.config.classifier.strategies.recent_turns') }}</option>
        </select>
      </label>
      <label v-for="field in numberFields" :key="field.key">
        <span class="input-label">{{ t(field.label) }}</span>
        <input v-model="overrides[field.key]" type="number" :min="field.min" :max="field.max" class="input" :placeholder="t('gptAuto.debugger.saved')" />
      </label>
    </div>

    <label class="block">
      <span class="input-label">{{ t('gptAuto.debugger.request') }}</span>
      <textarea v-model="requestText" rows="12" class="input font-mono text-xs" spellcheck="false" data-testid="gpt-auto-debug-request"></textarea>
    </label>

    <div class="flex items-center justify-end gap-3">
      <p v-if="error" role="alert" class="mr-auto text-sm text-red-600 dark:text-red-400" data-testid="gpt-auto-debug-error">{{ error }}</p>
      <button type="button" class="btn btn-primary" :disabled="running || !requestText.trim()" data-testid="gpt-auto-debug-run" @click="run">
        {{ running ? t('gptAuto.common.working') : t('gptAuto.debugger.run') }}
      </button>
    </div>

    <div v-if="result" class="space-y-4" data-testid="gpt-auto-debug-result">
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span class="rounded-full px-2 py-0.5" :class="result.classifiable ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'">
          {{ result.classifiable ? t('gptAuto.debugger.classifiable') : t('gptAuto.debugger.notClassifiable') }}
        </span>
        <span class="rounded-full bg-gray-100 px-2 py-0.5 dark:bg-dark-700">{{ result.context.state_version }} · {{ t(`gptAuto.config.classifier.strategies.${result.options.strategy}`) }}</span>
        <span class="rounded-full bg-gray-100 px-2 py-0.5 dark:bg-dark-700">{{ t(`gptAuto.debugger.kinds.${result.context.latest_user_kind}`) }}</span>
        <span v-for="limitation in result.context.limitations ?? []" :key="limitation" class="rounded-full bg-amber-50 px-2 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300" data-testid="gpt-auto-debug-limitation">
          {{ labels.limitation(limitation) }}
        </span>
      </div>
      <dl class="grid grid-cols-2 gap-3 text-xs md:grid-cols-4" data-testid="gpt-auto-debug-info">
        <div v-for="fact in facts" :key="fact.label">
          <dt class="text-gray-500 dark:text-gray-400">{{ fact.label }}</dt>
          <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ fact.value }}</dd>
        </div>
      </dl>
      <section class="space-y-3" data-testid="gpt-auto-debug-preview">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.debugger.previewTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.previewHint') }}</p>
        </div>
        <GPTAutoContextDiagnostics :context="result.context" :classifier-hash="result.classifier_config_hash" :classifier-schema="result.effective.schema_version" synthetic />
        <div data-testid="gpt-auto-debug-excerpts">
          <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.excerptsTitle') }}</p>
          <ul v-if="excerpts.length" class="space-y-1.5">
            <li v-for="(excerpt, index) in excerpts" :key="index" class="rounded bg-gray-50 px-3 py-1.5 text-xs dark:bg-dark-800">
              <p class="font-medium text-gray-700 dark:text-gray-300">{{ excerpt.label }}</p>
              <p class="whitespace-pre-wrap break-words text-gray-600 dark:text-gray-400">{{ excerpt.text }}</p>
            </li>
          </ul>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.noExcerpts') }}</p>
        </div>
      </section>
      <div>
        <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.state') }}</p>
        <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300" data-testid="gpt-auto-debug-state">{{ prettyState }}</pre>
      </div>
      <details class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600">
        <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ t('gptAuto.debugger.questions', { hash: result.classifier_config_hash.slice(0, 10) }) }}</summary>
        <div v-for="(question, id) in result.jev_request.questions" :key="id" class="mt-2">
          <p class="font-mono font-semibold">{{ id }} ({{ question.type }})</p>
          <p class="whitespace-pre-wrap text-gray-600 dark:text-gray-400">{{ question.instructions }}</p>
          <p class="mt-1 font-medium text-gray-500 dark:text-gray-400">{{ t('gptAuto.debugger.criteriaTitle') }}</p>
          <ul class="list-inside list-disc text-gray-600 dark:text-gray-400" :data-testid="`gpt-auto-debug-criteria-${id}`">
            <li v-for="(criterion, key) in question.criteria" :key="key"><span class="font-mono">{{ key }}</span>: {{ criterion }}</li>
          </ul>
        </div>
      </details>
      <details class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600">
        <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ t('gptAuto.debugger.rawRequest') }}</summary>
        <pre class="mt-2 max-h-96 overflow-auto whitespace-pre-wrap break-all">{{ JSON.stringify(result.jev_request, null, 2) }}</pre>
      </details>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GPTAutoContextDiagnostics from './GPTAutoContextDiagnostics.vue'
import { gptAutoAPI, type GPTAutoContextStrategy, type GPTAutoDebugResult, type GPTAutoEndpoint } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatNumber } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'
import { gptAutoDebugExample, gptAutoDebugExamples, type GPTAutoDebugExample } from '@/utils/gptAutoDebugExamples'

const { t } = useI18n()
const labels = useGPTAutoLabels()

const examples = gptAutoDebugExamples
const endpoint = ref<GPTAutoEndpoint>('responses')
const strategy = ref<GPTAutoContextStrategy | ''>('')
const requestText = ref(JSON.stringify(gptAutoDebugExample('agentFailure').request, null, 2))
const overrides = reactive<Record<'max_chars' | 'context_turns' | 'agent_steps', string | number>>({ max_chars: '', context_turns: '', agent_steps: '' })
const numberFields = [
  { key: 'max_chars' as const, label: 'gptAuto.config.classifier.maxChars', min: 100, max: 200000 },
  { key: 'context_turns' as const, label: 'gptAuto.config.classifier.contextMessages', min: 0, max: 20 },
  { key: 'agent_steps' as const, label: 'gptAuto.config.classifier.agentSteps', min: 1, max: 30 },
]
const running = ref(false)
const error = ref('')
const result = ref<GPTAutoDebugResult | null>(null)

function loadExample(name: GPTAutoDebugExample) {
  const example = gptAutoDebugExample(name)
  endpoint.value = example.endpoint
  requestText.value = JSON.stringify(example.request, null, 2)
  result.value = null
}

function numberOrUndefined(value: string | number): number | undefined {
  if (value === '' || value === null || value === undefined) return undefined
  const number = Number(value)
  return Number.isFinite(number) ? number : undefined
}

async function run() {
  error.value = ''
  let request: Record<string, unknown>
  try {
    const parsed = JSON.parse(requestText.value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('not an object')
    request = parsed as Record<string, unknown>
  } catch {
    error.value = t('gptAuto.debugger.invalidJson')
    return
  }
  running.value = true
  try {
    result.value = await gptAutoAPI.debugState({
      endpoint: endpoint.value,
      request,
      strategy: strategy.value || undefined,
      max_chars: numberOrUndefined(overrides.max_chars),
      context_turns: numberOrUndefined(overrides.context_turns),
      agent_steps: numberOrUndefined(overrides.agent_steps),
    })
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAuto.debugger.failed'))
  } finally {
    running.value = false
  }
}

const prettyState = computed(() => {
  try {
    return JSON.stringify(JSON.parse(result.value?.jev_request.state ?? ''), null, 2)
  } catch {
    return result.value?.jev_request.state ?? ''
  }
})

interface StateExcerptSource {
  latest_user_request?: string
  task_context?: { continues_request?: string; original_request?: string; history_summary?: string }
  recent_context?: { role: string; text: string }[]
  agent_activity?: { recent?: { kind: string; status?: string; summary?: string }[] }
}

/** The excerpts exactly as they appear in the server-built state. */
const excerpts = computed(() => {
  let state: StateExcerptSource
  try {
    state = JSON.parse(result.value?.jev_request.state ?? '') as StateExcerptSource
  } catch {
    return []
  }
  const out: { label: string; text: string }[] = []
  const add = (label: string, text?: string) => {
    if (text) out.push({ label, text })
  }
  add(t('gptAuto.debugger.excerptLatest'), state.latest_user_request)
  add(t('gptAuto.debugger.excerptContinues'), state.task_context?.continues_request)
  for (const event of state.agent_activity?.recent ?? []) {
    add(t('gptAuto.debugger.excerptEvent', { kind: event.status ? `${event.kind} (${event.status})` : event.kind }), event.summary)
  }
  add(t('gptAuto.debugger.excerptOriginal'), state.task_context?.original_request)
  add(t('gptAuto.debugger.excerptSummary'), state.task_context?.history_summary)
  for (const turn of state.recent_context ?? []) add(t('gptAuto.debugger.excerptRecent', { role: turn.role }), turn.text)
  return out
})

const facts = computed(() => {
  const value = result.value
  if (!value) return []
  const info = value.context
  const index = (n: number) => (n >= 0 ? `#${n}` : '—')
  return [
    { label: t('gptAuto.debugger.facts.items'), value: formatNumber(info.items_total) },
    { label: t('gptAuto.debugger.facts.latestUser'), value: `${index(info.latest_user_index)}${info.latest_user_truncated ? ` · ${t('gptAuto.debugger.truncated')}` : ''}` },
    { label: t('gptAuto.debugger.facts.anchor'), value: index(info.follow_up_anchor_index) },
    { label: t('gptAuto.debugger.facts.original'), value: index(info.original_request_index) },
    { label: t('gptAuto.debugger.facts.turns'), value: `${info.context_turns_included} / ${info.context_turns_available}` },
    { label: t('gptAuto.debugger.facts.events'), value: `${info.agent_events_included} (+${info.agent_events_omitted})` },
    { label: t('gptAuto.debugger.facts.budget'), value: `${formatNumber(info.used_chars)} / ${formatNumber(info.budget_chars)}` },
    { label: t('gptAuto.debugger.facts.sections'), value: Object.entries(info.section_chars ?? {}).map(([k, v]) => `${k} ${v}`).join(', ') || '—' },
    { label: t('gptAuto.debugger.facts.stateChars'), value: formatNumber(value.state_chars) },
    { label: t('gptAuto.debugger.facts.jevTokens'), value: formatNumber(value.estimated_jev_input_tokens) },
    { label: t('gptAuto.debugger.facts.requestTokens'), value: formatNumber(value.estimated_input_tokens) },
    { label: t('gptAuto.debugger.facts.scanned'), value: formatNumber(info.scanned_bytes) },
  ]
})

defineExpose({ loadExample })
</script>
