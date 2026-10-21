<template>
  <div class="space-y-4" data-testid="gpt-auto-lab">
    <div class="rounded-lg border border-primary-200 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-800/60 dark:bg-primary-900/20 dark:text-primary-200">
      {{ t('gptAuto.lab.notice') }}
    </div>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <section class="card space-y-4 p-6">
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

        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <label>
            <span class="input-label">{{ t('gptAuto.lab.endpoint') }}</span>
            <select v-model="endpoint" class="input">
              <option value="chat_completions">{{ labels.endpoint('chat_completions') }}</option>
              <option value="responses">{{ labels.endpoint('responses') }}</option>
            </select>
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.lab.inputTokens') }}</span>
            <input v-model="inputTokensText" type="number" min="0" class="input" :placeholder="t('gptAuto.lab.inputTokensPlaceholder')" />
          </label>
          <label class="flex items-end gap-2 pb-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="hasTools" type="checkbox" class="rounded border-gray-300" />
            {{ t('gptAuto.lab.hasTools') }}
          </label>
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

        <div class="flex items-center justify-end gap-3">
          <p v-if="error" role="alert" class="mr-auto text-sm text-red-600 dark:text-red-400" data-testid="gpt-auto-lab-error">{{ error }}</p>
          <button type="button" class="btn btn-primary" :disabled="running || !prompt.trim()" data-testid="gpt-auto-lab-run" @click="run">
            {{ running ? t('gptAuto.lab.running') : t('gptAuto.lab.analyze') }}
          </button>
        </div>
      </section>

      <section class="card p-6">
        <h2 class="mb-3 text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.lab.result') }}</h2>
        <p v-if="!result" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.lab.resultEmpty') }}</p>
        <div v-else class="space-y-4" data-testid="gpt-auto-lab-result">
          <div class="flex flex-wrap gap-4 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('gptAuto.detail.classifier') }}: {{ result.classification.classifier_model }}</span>
            <span>{{ t('gptAuto.detail.latency') }}: {{ formatMs(result.latency_ms) }}</span>
            <span>{{ t('gptAuto.detail.jevTokens') }}: {{ formatNumber(result.classification.input_tokens) }} / {{ formatNumber(result.classification.output_tokens) }}</span>
            <span>{{ t('gptAuto.detail.jevCost') }}: {{ formatUSD(result.classifier_cost, 8) }}</span>
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GPTAutoDecisionDetail from './GPTAutoDecisionDetail.vue'
import { gptAutoAPI, type GPTAutoContextTurn, type GPTAutoEndpoint, type GPTAutoModelProfile, type GPTAutoTestResult } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { cloneProfiles, formatMs, formatNumber, formatUSD } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const { t } = useI18n()
const labels = useGPTAutoLabels()

const prompt = ref('')
const context = ref<GPTAutoContextTurn[]>([])
const endpoint = ref<GPTAutoEndpoint>('chat_completions')
const hasTools = ref(false)
const inputTokensText = ref('')
const useConstraints = ref(false)
const profiles = ref<GPTAutoModelProfile[]>([])
const running = ref(false)
const error = ref('')
const result = ref<GPTAutoTestResult | null>(null)

const prettySnapshot = computed(() => {
  try {
    return JSON.stringify(JSON.parse(result.value?.snapshot_preview ?? ''), null, 2)
  } catch {
    return result.value?.snapshot_preview ?? ''
  }
})

async function loadProfiles() {
  try {
    const config = await gptAutoAPI.getConfig()
    profiles.value = cloneProfiles(config.model_profiles ?? [])
  } catch {
    profiles.value = []
  }
}

async function run() {
  running.value = true
  error.value = ''
  try {
    const tokens = inputTokensText.value === '' ? undefined : Number(inputTokensText.value)
    result.value = await gptAutoAPI.runTest({
      prompt: prompt.value,
      context: context.value.filter((turn) => turn.text.trim()),
      endpoint: endpoint.value,
      has_tools: hasTools.value,
      model_profiles: useConstraints.value ? cloneProfiles(profiles.value) : undefined,
      estimated_input_tokens: tokens !== undefined && Number.isFinite(tokens) && tokens >= 0 ? tokens : undefined,
    })
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('gptAuto.lab.failed'))
  } finally {
    running.value = false
  }
}

onMounted(() => { void loadProfiles() })

defineExpose({ loadProfiles })
</script>
