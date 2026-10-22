<template>
  <div class="space-y-4" data-testid="gpt-auto-lab">
    <div class="rounded-lg border border-primary-200 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-800/60 dark:bg-primary-900/20 dark:text-primary-200">
      {{ t('gptAuto.lab.notice') }}
    </div>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <section class="card space-y-4 p-6">
        <label v-if="presets.length" class="block">
          <span class="input-label">{{ t('gptAuto.lab.preset') }}</span>
          <select v-model="presetId" class="input" data-testid="gpt-auto-lab-preset" @change="applyPreset">
            <option value="">{{ t('gptAuto.lab.presetNone') }}</option>
            <optgroup v-for="group in presetGroups" :key="group" :label="labels.benchmarkGroup(group)">
              <option v-for="sample in presets.filter((p) => p.group === group)" :key="sample.id" :value="sample.id">
                {{ sample.id }} · {{ t('gptAuto.lab.presetExpect', { tier: sample.expected_tier }) }}
              </option>
            </optgroup>
          </select>
          <span class="input-hint">{{ t('gptAuto.lab.presetHint') }}</span>
        </label>

        <label class="block">
          <span class="input-label">{{ t('gptAuto.lab.prompt') }}</span>
          <textarea v-model="prompt" rows="6" maxlength="50000" class="input font-mono text-sm" :placeholder="t('gptAuto.lab.promptPlaceholder')" data-testid="gpt-auto-lab-prompt"></textarea>
        </label>

        <div>
          <div class="flex items-center justify-between">
            <span class="input-label">{{ t('gptAuto.lab.context') }}</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="context.length >= 20" @click="context.push({ role: 'user', text: '' })">{{ t('gptAuto.lab.addTurn') }}</button>
          </div>
          <p class="input-hint">{{ t('gptAuto.lab.contextHint') }}</p>
          <div v-for="(turn, index) in context" :key="index" class="mt-2 flex gap-2">
            <select v-model="turn.role" class="input w-32">
              <option value="system">system</option>
              <option value="user">user</option>
              <option value="assistant">assistant</option>
              <option value="tool">tool</option>
            </select>
            <textarea v-model="turn.text" rows="2" class="input flex-1 text-sm"></textarea>
            <button type="button" class="btn btn-ghost btn-sm" @click="context.splice(index, 1)">{{ t('gptAuto.common.remove') }}</button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <label>
            <span class="input-label">{{ t('gptAuto.lab.endpoint') }}</span>
            <select v-model="endpoint" class="input">
              <option value="chat_completions">{{ labels.endpoint('chat_completions') }}</option>
              <option value="responses">{{ labels.endpoint('responses') }}</option>
            </select>
          </label>
          <label class="flex items-end gap-2 pb-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="hasTools" type="checkbox" class="rounded border-gray-300" />
            {{ t('gptAuto.lab.hasTools') }}
          </label>
        </div>

        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="gpt-auto-lab-assumptions">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.lab.assumptions') }}</p>
          <p class="input-hint">{{ t('gptAuto.lab.assumptionsHint') }}</p>
          <div class="mt-2 grid grid-cols-1 gap-3 md:grid-cols-3">
            <label v-for="field in assumptionFields" :key="field">
              <span class="input-label">{{ t(`gptAuto.lab.assumptionFields.${field}`) }}</span>
              <input v-model="assumptions[field]" type="number" min="0" max="10000000" step="1" class="input" :placeholder="t('gptAuto.lab.assumptionPlaceholder')" :data-testid="`gpt-auto-lab-assume-${field}`" />
            </label>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <label class="flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
            <input v-model="useConstraints" type="checkbox" class="rounded border-gray-300" :disabled="!profiles.length" data-testid="gpt-auto-lab-constraints" />
            {{ t('gptAuto.lab.constraints') }}
          </label>
          <p class="input-hint">{{ t('gptAuto.lab.constraintsHint') }}</p>
          <div v-if="useConstraints" class="mt-2 space-y-2">
            <div v-for="profile in profiles" :key="profile.model" class="flex flex-wrap items-center gap-4 text-sm">
              <span class="w-28 font-mono">{{ profile.model }}</span>
              <label class="flex items-center gap-1"><input v-model="profile.enabled" type="checkbox" class="rounded border-gray-300" />{{ t('gptAuto.common.enabled') }}</label>
              <label class="flex items-center gap-1"><input v-model="profile.supports_tools" type="checkbox" class="rounded border-gray-300" />{{ t('gptAuto.config.models.supportsTools') }}</label>
              <label class="flex items-center gap-1">{{ t('gptAuto.config.models.routingWeight') }}<input v-model.number="profile.routing_weight" type="number" min="0" max="1000" class="input w-20" /></label>
              <label class="flex items-center gap-1">{{ t('gptAuto.config.models.billingMultiplier') }}<input v-model.number="profile.billing_multiplier" type="number" min="0.01" max="100" step="0.01" class="input w-20" /></label>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="gpt-auto-lab-compare">
          <label class="flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
            <input v-model="compareMode" type="checkbox" class="rounded border-gray-300" data-testid="gpt-auto-lab-compare-toggle" />
            {{ t('gptAuto.lab.compare') }}
          </label>
          <p class="input-hint">{{ t('gptAuto.lab.compareHint') }}</p>
          <template v-if="compareMode">
            <p class="mt-2 rounded bg-amber-50 px-2 py-1.5 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200" data-testid="gpt-auto-lab-compare-cost">{{ t('gptAuto.lab.compareCost') }}</p>
            <p v-if="draftError" class="mt-2 text-xs text-red-600">{{ draftError }}</p>
            <div v-for="field in instructionFields" :key="field" class="mt-3">
              <span class="input-label">{{ t(`gptAuto.classifier.fields.${field}`) }}</span>
              <textarea v-model="draft[field]" rows="4" maxlength="4000" class="input text-xs" :data-testid="`gpt-auto-lab-draft-${field}`"></textarea>
            </div>
          </template>
        </div>

        <div class="flex items-center justify-end gap-3">
          <p v-if="error" role="alert" class="mr-auto text-sm text-red-600 dark:text-red-400" data-testid="gpt-auto-lab-error">{{ error }}</p>
          <button type="button" class="btn btn-primary" :disabled="running || !prompt.trim() || !assumptionsValid" data-testid="gpt-auto-lab-run" @click="run">
            {{ running ? t('gptAuto.lab.running') : compareMode ? t('gptAuto.lab.runCompare') : t('gptAuto.lab.analyze') }}
          </button>
        </div>
      </section>

      <section class="card p-6">
        <h2 class="mb-3 text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.lab.result') }}</h2>
        <p v-if="!result && !comparison" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.lab.resultEmpty') }}</p>

        <div v-else-if="comparison" class="space-y-4" data-testid="gpt-auto-lab-compare-result">
          <p class="text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-auto-lab-compare-summary">
            {{ t('gptAuto.lab.compareSummary', { calls: comparison.jev_calls, cost: formatUSD(comparison.total_classifier_cost) }) }}
          </p>
          <div class="overflow-x-auto">
            <table class="min-w-full text-xs">
              <thead>
                <tr class="text-left text-gray-500 dark:text-gray-400">
                  <th class="py-1 pr-3"></th>
                  <th class="py-1 pr-3">{{ t('gptAuto.lab.armSaved') }}</th>
                  <th class="py-1">{{ t('gptAuto.lab.armDraft') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in comparisonRows" :key="row.key" class="border-t border-gray-100 dark:border-dark-700" :data-testid="`gpt-auto-compare-row-${row.key}`">
                  <td class="py-1 pr-3 text-gray-500">{{ row.label }}</td>
                  <td class="py-1 pr-3 font-mono" :class="row.changed ? 'text-amber-700 dark:text-amber-300' : ''">{{ row.saved }}</td>
                  <td class="py-1 font-mono" :class="row.changed ? 'text-amber-700 dark:text-amber-300' : ''">{{ row.draft }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <details v-for="arm in compareArms" :key="arm.key" class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600">
            <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ arm.label }}</summary>
            <GPTAutoDecisionDetail v-if="arm.side.result" class="mt-2" :classification="arm.side.result.classification" :decision="arm.side.result.decision" />
            <p v-else class="mt-2 text-red-600">{{ arm.side.error_message }}</p>
          </details>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.lab.compareNotSaved') }}</p>
        </div>

        <div v-else-if="result" class="space-y-4" data-testid="gpt-auto-lab-result">
          <div class="flex flex-wrap gap-4 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('gptAuto.detail.classifier') }}: {{ result.classification.classifier_model }}</span>
            <span>{{ t('gptAuto.detail.latency') }}: {{ formatMs(result.latency_ms) }}</span>
            <span>{{ t('gptAuto.detail.jevTokens') }}: {{ formatNumber(result.classifier_input_tokens ?? result.classification.input_tokens) }} / {{ formatNumber(result.classifier_output_tokens ?? result.classification.output_tokens) }}</span>
            <span>{{ t('gptAuto.detail.jevCost') }}: {{ formatUSD(result.classifier_cost) }}</span>
            <span v-if="result.classifier_config_hash">{{ t('gptAuto.detail.classifierConfig') }}: <span class="font-mono">{{ result.classifier_config_hash.slice(0, 10) }}</span></span>
            <span v-if="result.profiles_overridden" class="text-amber-600">{{ t('gptAuto.lab.overridden') }}</span>
          </div>
          <GPTAutoDecisionDetail :classification="result.classification" :decision="result.decision" />
          <details class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600">
            <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ t('gptAuto.lab.snapshot') }}</summary>
            <pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all text-gray-600 dark:text-gray-400">{{ prettySnapshot }}</pre>
          </details>
        </div>
      </section>
    </div>

    <GPTAutoStateDebugger />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import GPTAutoDecisionDetail from './GPTAutoDecisionDetail.vue'
import GPTAutoStateDebugger from './GPTAutoStateDebugger.vue'
import {
  gptAutoAPI, gptAutoInstructionFields,
  type GPTAutoBenchmarkSample, type GPTAutoCompareResult, type GPTAutoCompareSide, type GPTAutoContextTurn, type GPTAutoEndpoint,
  type GPTAutoInstructionField, type GPTAutoModelProfile, type GPTAutoTestResult, type GPTAutoWorkloadAssumptions,
} from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { cloneProfiles, formatMs, formatNumber, formatPercent, formatUSD, sortedProbabilities } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const { t } = useI18n()
const labels = useGPTAutoLabels()
const instructionFields = gptAutoInstructionFields

const prompt = ref('')
const context = ref<GPTAutoContextTurn[]>([])
const endpoint = ref<GPTAutoEndpoint>('chat_completions')
const hasTools = ref(false)
const useConstraints = ref(false)
const profiles = ref<GPTAutoModelProfile[]>([])
const running = ref(false)
const error = ref('')
const result = ref<GPTAutoTestResult | null>(null)
const comparison = ref<GPTAutoCompareResult | null>(null)
const presets = ref<GPTAutoBenchmarkSample[]>([])
const presetId = ref('')
const compareMode = ref(false)
const draftLoaded = ref(false)
const draftError = ref('')
const draft = reactive<Record<GPTAutoInstructionField, string>>({ model_selection: '', effort_selection: '', complexity_scoring: '', category_classification: '' })

type AssumptionField = keyof GPTAutoWorkloadAssumptions
const assumptionFields: AssumptionField[] = ['input_tokens', 'cached_input_tokens', 'output_tokens']
const assumptions = reactive<Record<AssumptionField, string | number>>({ input_tokens: '', cached_input_tokens: '', output_tokens: '' })

const presetGroups = computed(() => [...new Set(presets.value.map((sample) => sample.group))])

function parseAssumption(value: string | number): number | undefined | null {
  if (value === '' || value === null || value === undefined) return undefined
  const number = Number(value)
  return Number.isInteger(number) && number >= 0 && number <= 10_000_000 ? number : null
}
const assumptionsValid = computed(() => assumptionFields.every((field) => parseAssumption(assumptions[field]) !== null))
function buildAssumptions(): GPTAutoWorkloadAssumptions | undefined {
  const out: GPTAutoWorkloadAssumptions = {}
  for (const field of assumptionFields) {
    const value = parseAssumption(assumptions[field])
    if (typeof value === 'number') out[field] = value
  }
  return Object.keys(out).length ? out : undefined
}

const prettySnapshot = computed(() => {
  try {
    return JSON.stringify(JSON.parse(result.value?.snapshot_preview ?? ''), null, 2)
  } catch {
    return result.value?.snapshot_preview ?? ''
  }
})

function applyPreset() {
  const sample = presets.value.find((entry) => entry.id === presetId.value)
  if (!sample) return
  prompt.value = sample.prompt
  context.value = (sample.context ?? []).map((turn) => ({ ...turn }))
  hasTools.value = sample.has_tools
}

async function loadProfiles() {
  try {
    const [config, meta] = await Promise.all([gptAutoAPI.getConfig(), gptAutoAPI.getMeta()])
    profiles.value = cloneProfiles(config.model_profiles ?? [])
    presets.value = meta.benchmark_samples ?? []
  } catch {
    profiles.value = []
  }
}

async function ensureDraft() {
  if (!compareMode.value || draftLoaded.value) return
  draftError.value = ''
  try {
    const view = await gptAutoAPI.getClassifier()
    for (const field of instructionFields) draft[field] = view.instructions[field] ?? ''
    draftLoaded.value = true
  } catch (err) {
    draftError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  }
}

watch(compareMode, () => { void ensureDraft() })

function payload() {
  return {
    prompt: prompt.value,
    context: context.value.filter((turn) => turn.text.trim()),
    endpoint: endpoint.value,
    has_tools: hasTools.value,
    model_profiles: useConstraints.value ? cloneProfiles(profiles.value) : undefined,
    assumptions: buildAssumptions(),
  }
}

async function run() {
  running.value = true
  error.value = ''
  try {
    if (compareMode.value) {
      result.value = null
      comparison.value = await gptAutoAPI.compareClassifier({ ...payload(), draft_instructions: { ...draft } })
    } else {
      comparison.value = null
      result.value = await gptAutoAPI.runTest(payload())
    }
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAuto.lab.failed'))
  } finally {
    running.value = false
  }
}

const compareArms = computed(() => comparison.value
  ? [
      { key: 'saved', label: t('gptAuto.lab.armSaved'), side: comparison.value.saved },
      { key: 'draft', label: t('gptAuto.lab.armDraft'), side: comparison.value.draft },
    ]
  : [])

function describe(side: GPTAutoCompareSide, key: string): string {
  const out = side.result
  if (!out) return key === 'model' ? (side.error_message || side.error_code || '—') : '—'
  const cls = out.classification
  switch (key) {
    case 'model': return cls.model
    case 'final': return `${out.decision.final_model || '—'} / ${labels.effort(out.decision.final_effort)}`
    case 'effort': return labels.effort(cls.effort)
    case 'complexity': return `${cls.complexity.toFixed(2)} / 4`
    case 'category': return labels.category(cls.category)
    case 'probabilities': return sortedProbabilities(cls.model_probabilities).map(([model, p]) => `${model} ${formatPercent(p, 0)}`).join(', ') || '—'
    case 'confidence': return `${formatPercent(cls.model_confidence, 0)} · ${formatPercent(cls.effort_confidence, 0)}`
    case 'latency': return formatMs(out.latency_ms)
    case 'tokens': return `${formatNumber(out.classifier_input_tokens ?? cls.input_tokens)} / ${formatNumber(out.classifier_output_tokens ?? cls.output_tokens)}`
    case 'cost': return formatUSD(out.classifier_cost)
    case 'hash': return side.classifier_config_hash.slice(0, 10)
    default: return '—'
  }
}

const comparisonRows = computed(() => {
  const value = comparison.value
  if (!value) return []
  const keys = ['model', 'final', 'effort', 'complexity', 'category', 'probabilities', 'confidence', 'latency', 'tokens', 'cost', 'hash']
  return keys.map((key) => {
    const saved = describe(value.saved, key)
    const draftValue = describe(value.draft, key)
    const compared = ['model', 'final', 'effort', 'category'].includes(key)
    return { key, label: t(`gptAuto.lab.compareRows.${key}`), saved, draft: draftValue, changed: compared && saved !== draftValue }
  })
})

onMounted(() => { void loadProfiles() })

defineExpose({ loadProfiles })
</script>
