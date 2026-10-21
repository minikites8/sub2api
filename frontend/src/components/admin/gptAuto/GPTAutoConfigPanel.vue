<template>
  <div class="space-y-6" data-testid="gpt-auto-config">
    <div v-if="loading" class="flex justify-center py-12">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>
    <p v-else-if="loadError" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ loadError }}</p>

    <template v-else-if="form && saved">
      <div class="rounded-lg border border-primary-200 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-800/60 dark:bg-primary-900/20 dark:text-primary-200">
        {{ t('gptAuto.config.shadowOnlyNotice') }}
      </div>

      <!-- Sampling -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.config.sampling.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.sampling.hint') }}</p>
        </header>
        <div class="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-3">
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.config.sampling.enabled') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.sampling.enabledHint') }}</p>
            </div>
            <Toggle v-model="form.shadow_enabled" data-testid="gpt-auto-shadow-toggle" />
          </div>
          <label>
            <span class="input-label">{{ t('gptAuto.config.sampling.sampleRate') }}</span>
            <input v-model.number="form.sample_rate" type="number" min="0" max="100" step="1" class="input" data-testid="gpt-auto-sample-rate" />
            <span class="input-hint">{{ t('gptAuto.config.sampling.sampleRateHint') }}</span>
          </label>
          <div>
            <span class="input-label">{{ t('gptAuto.config.sampling.endpoints') }}</span>
            <div class="mt-2 flex flex-wrap gap-4">
              <label v-for="endpoint in endpointOptions" :key="endpoint" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="form.endpoints" type="checkbox" :value="endpoint" class="rounded border-gray-300" />
                {{ labels.endpoint(endpoint) }}
              </label>
            </div>
            <span class="input-hint">{{ t('gptAuto.config.sampling.endpointsHint') }}</span>
          </div>
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.config.sampling.allGroups') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.sampling.allGroupsHint') }}</p>
            </div>
            <Toggle v-model="form.all_groups" />
          </div>
          <label v-if="!form.all_groups">
            <span class="input-label">{{ t('gptAuto.config.sampling.groupIds') }}</span>
            <input v-model="groupIdsText" type="text" class="input" placeholder="1, 2, 3" />
            <span class="input-hint">{{ t('gptAuto.config.sampling.groupIdsHint') }}</span>
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.config.sampling.modelFilter') }}</span>
            <input v-model="modelFilterText" type="text" class="input" placeholder="gpt-5.6-sol, gpt-6-luna" />
            <span class="input-hint">{{ t('gptAuto.config.sampling.modelFilterHint') }}</span>
          </label>
        </div>
      </section>

      <!-- Classifier -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.config.classifier.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.classifier.hint') }}</p>
        </header>
        <div class="space-y-4 p-6">
          <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200" data-testid="gpt-auto-disclosure">
            {{ t('gptAuto.config.classifier.disclosure') }}
          </div>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            <label>
              <span class="input-label">{{ t('gptAuto.config.classifier.baseUrl') }}</span>
              <input v-model.trim="form.classifier_base_url" type="url" class="input" placeholder="https://api.typesafe.ai" />
            </label>
            <label>
              <span class="input-label">{{ t('gptAuto.config.classifier.apiKey') }}</span>
              <input v-model="form.classifier_api_key" type="password" autocomplete="new-password" class="input" :placeholder="saved.classifier_api_key_configured ? saved.classifier_api_key_masked : t('gptAuto.config.classifier.apiKeyEmpty')" data-testid="gpt-auto-api-key" />
              <span class="input-hint">{{ saved.classifier_api_key_configured ? t('gptAuto.config.classifier.apiKeyKeep') : t('gptAuto.config.classifier.apiKeyRequired') }}</span>
              <label v-if="saved.classifier_api_key_configured" class="mt-1 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                <input v-model="form.clear_classifier_api_key" type="checkbox" class="rounded border-gray-300" />
                {{ t('gptAuto.config.classifier.clearKey') }}
              </label>
            </label>
            <label>
              <span class="input-label">{{ t('gptAuto.config.classifier.model') }}</span>
              <input v-model.trim="form.classifier_model" type="text" class="input" />
            </label>
            <label>
              <span class="input-label">{{ t('gptAuto.config.classifier.proxyId') }}</span>
              <input v-model.number="proxyIdInput" type="number" min="0" class="input" />
              <span class="input-hint">{{ t('gptAuto.config.classifier.proxyIdHint') }}</span>
            </label>
            <label v-for="field in numericFields" :key="field.key">
              <span class="input-label">{{ t(field.label) }}</span>
              <input v-model.number="(form as unknown as Record<string, number>)[field.key]" type="number" :min="field.min" :max="field.max" :step="field.step ?? 1" class="input" />
              <span v-if="field.hint" class="input-hint">{{ t(field.hint) }}</span>
            </label>
          </div>
        </div>
      </section>

      <!-- Margins -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.config.margin.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.margin.hint') }}</p>
        </header>
        <div class="grid grid-cols-1 gap-4 p-6 md:grid-cols-3">
          <label>
            <span class="input-label">{{ t('gptAuto.config.margin.globalMin') }}</span>
            <input :value="toPct(form.global_min_margin)" type="number" min="0" max="99" step="0.5" class="input" data-testid="gpt-auto-global-margin" @input="form.global_min_margin = fromPct($event) ?? 0" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.config.margin.buffer') }}</span>
            <input :value="toPct(form.profit_safety_buffer)" type="number" min="0" max="99" step="0.5" class="input" @input="form.profit_safety_buffer = fromPct($event) ?? 0" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.config.margin.policyVersion') }}</span>
            <input v-model.trim="form.policy_version" type="text" maxlength="64" class="input" />
            <span class="input-hint">{{ t('gptAuto.config.margin.policyVersionHint') }}</span>
          </label>
        </div>
      </section>

      <!-- Model table -->
      <section class="card overflow-hidden">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.config.models.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.config.models.hint') }}</p>
        </header>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <div v-for="profile in form.model_profiles" :key="profile.model" class="space-y-4 p-6" :data-testid="`gpt-auto-profile-${profile.model}`">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <Toggle v-model="profile.enabled" />
                <h3 class="font-mono text-base font-semibold text-gray-900 dark:text-white">{{ profile.model }}</h3>
              </div>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="profile.supports_tools" type="checkbox" class="rounded border-gray-300" />
                {{ t('gptAuto.config.models.supportsTools') }}
              </label>
            </div>
            <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
              <label>
                <span class="input-label">{{ t('gptAuto.config.models.billingMultiplier') }}</span>
                <input v-model.number="profile.billing_multiplier" type="number" min="0.01" max="100" step="0.01" class="input" :class="profileErrors[profile.model]?.multiplier ? 'input-error' : ''" :data-testid="`gpt-auto-multiplier-${profile.model}`" />
                <span class="input-hint">{{ t('gptAuto.config.models.billingMultiplierHint') }}</span>
              </label>
              <label>
                <span class="input-label">{{ t('gptAuto.config.models.routingWeight') }}</span>
                <input v-model.number="profile.routing_weight" type="number" min="0" max="1000" step="1" class="input" :class="profileErrors[profile.model]?.weight ? 'input-error' : ''" :data-testid="`gpt-auto-weight-${profile.model}`" />
                <span class="input-hint">{{ t('gptAuto.config.models.routingWeightHint') }}</span>
              </label>
              <label>
                <span class="input-label">{{ t('gptAuto.config.models.minMargin') }}</span>
                <input :value="toPct(profile.min_margin)" type="number" min="0" max="99" step="0.5" class="input" :placeholder="t('gptAuto.config.models.minMarginPlaceholder', { value: toPct(form.global_min_margin) })" @input="profile.min_margin = fromPct($event)" />
              </label>
            </div>
            <div>
              <span class="input-label">{{ t('gptAuto.config.models.efforts') }}</span>
              <div class="mt-2 flex flex-wrap gap-3">
                <label v-for="effort in effortLadder" :key="effort" class="flex items-center gap-1.5 text-sm" :class="nativeEfforts(profile.model).includes(effort) ? 'text-gray-700 dark:text-gray-300' : 'text-gray-400 line-through dark:text-gray-600'" :title="nativeEfforts(profile.model).includes(effort) ? '' : t('gptAuto.config.models.effortUnsupported')">
                  <input v-model="profile.supported_efforts" type="checkbox" :value="effort" :disabled="!nativeEfforts(profile.model).includes(effort)" class="rounded border-gray-300" />
                  {{ labels.effort(effort) }}
                </label>
              </div>
              <span class="input-hint">{{ t('gptAuto.config.models.effortsHint') }}</span>
              <p v-if="profileErrors[profile.model]?.efforts" class="input-error-text">{{ t('gptAuto.config.models.effortsRequired') }}</p>
            </div>
            <label class="block">
              <span class="input-label">{{ t('gptAuto.config.models.description') }}</span>
              <textarea v-model="profile.description" rows="2" maxlength="500" class="input"></textarea>
              <span class="input-hint">{{ t('gptAuto.config.models.descriptionHint') }}</span>
            </label>
            <div>
              <div class="flex items-center justify-between">
                <span class="input-label">{{ t('gptAuto.config.models.upstreamCosts') }}</span>
                <button type="button" class="btn btn-secondary btn-sm" :disabled="(profile.upstream_costs?.length ?? 0) >= 16" @click="addChannel(profile)">{{ t('gptAuto.config.models.addChannel') }}</button>
              </div>
              <p class="input-hint">{{ t('gptAuto.config.models.upstreamCostsHint') }}</p>
              <div v-if="profile.upstream_costs?.length" class="mt-2 overflow-x-auto">
                <table class="min-w-full text-sm">
                  <thead>
                    <tr class="text-left text-xs text-gray-500 dark:text-gray-400">
                      <th class="py-1 pr-2">{{ t('gptAuto.config.models.channelName') }}</th>
                      <th class="py-1 pr-2">{{ t('gptAuto.config.models.inputPerM') }}</th>
                      <th class="py-1 pr-2">{{ t('gptAuto.config.models.outputPerM') }}</th>
                      <th class="py-1 pr-2">{{ t('gptAuto.config.models.cachedPerM') }}</th>
                      <th class="py-1 pr-2">{{ t('gptAuto.common.enabled') }}</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(cost, index) in profile.upstream_costs" :key="index">
                      <td class="py-1 pr-2"><input v-model.trim="cost.channel" type="text" maxlength="100" class="input" :class="!cost.channel ? 'input-error' : ''" /></td>
                      <td class="py-1 pr-2"><input v-model.number="cost.input_per_m" type="number" min="0" step="0.01" class="input w-28" /></td>
                      <td class="py-1 pr-2"><input v-model.number="cost.output_per_m" type="number" min="0" step="0.01" class="input w-28" /></td>
                      <td class="py-1 pr-2"><input v-model.number="cost.cached_input_per_m" type="number" min="0" step="0.01" class="input w-28" /></td>
                      <td class="py-1 pr-2"><input v-model="cost.enabled" type="checkbox" class="rounded border-gray-300" /></td>
                      <td class="py-1"><button type="button" class="btn btn-ghost btn-sm" @click="profile.upstream_costs?.splice(index, 1)">{{ t('gptAuto.common.remove') }}</button></td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Simulator -->
      <section class="card" data-testid="gpt-auto-simulator">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.simulator.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.simulator.hint') }}</p>
        </header>
        <div class="space-y-4 p-6">
          <div class="grid grid-cols-1 gap-4 md:grid-cols-5">
            <label>
              <span class="input-label">{{ t('gptAuto.simulator.effort') }}</span>
              <select v-model="sim.effort" class="input">
                <option v-for="effort in effortLadder" :key="effort" :value="effort">{{ labels.effort(effort) }}</option>
              </select>
            </label>
            <label>
              <span class="input-label">{{ t('gptAuto.simulator.inputTokens') }}</span>
              <input v-model.number="sim.input_tokens" type="number" min="0" class="input" />
            </label>
            <label>
              <span class="input-label">{{ t('gptAuto.simulator.outputTokens') }}</span>
              <input v-model="sim.outputTokensText" type="number" min="0" class="input" :placeholder="t('gptAuto.simulator.outputTokensPlaceholder')" />
            </label>
            <label class="flex items-end gap-2 pb-2 text-sm text-gray-700 dark:text-gray-300">
              <input v-model="sim.include_classifier_cost" type="checkbox" class="rounded border-gray-300" />
              {{ t('gptAuto.simulator.includeClassifier') }}
            </label>
            <div class="flex items-end">
              <button type="button" class="btn btn-primary w-full" :disabled="simLoading || hasErrors" data-testid="gpt-auto-simulate" @click="runSimulation">
                {{ simLoading ? t('gptAuto.common.working') : t('gptAuto.simulator.run') }}
              </button>
            </div>
          </div>
          <p v-if="simError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ simError }}</p>
          <div v-if="simResult" class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th v-for="column in simColumns" :key="column" class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(`gptAuto.simulator.columns.${column}`) }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="row in simResult.rows" :key="row.model" :class="row.below_target ? 'bg-red-50/60 dark:bg-red-900/10' : ''" :data-testid="`gpt-auto-sim-row-${row.model}`">
                  <td class="px-3 py-2 font-mono">{{ row.model }}</td>
                  <td class="px-3 py-2 tabular-nums">{{ row.billing_multiplier }}×</td>
                  <td class="px-3 py-2 tabular-nums">{{ row.pricing_available ? formatUSD(row.estimated_revenue) : '—' }}</td>
                  <td class="px-3 py-2 tabular-nums">
                    {{ row.pricing_available ? formatUSD(row.estimated_upstream_cost) : '—' }}
                    <span class="block text-[11px] text-gray-500">{{ row.upstream_channel || labels.costSource(row.upstream_cost_source) }}</span>
                  </td>
                  <td class="px-3 py-2 tabular-nums">{{ formatUSD(row.estimated_classifier_cost, 8) }}</td>
                  <td class="px-3 py-2 tabular-nums">{{ row.pricing_available ? formatUSD(row.estimated_gross_profit) : '—' }}</td>
                  <td class="px-3 py-2 tabular-nums" :class="row.below_target ? 'font-semibold text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
                    {{ formatPercent(row.estimated_gross_margin) }}
                    <span class="block text-[11px] font-normal text-gray-500">{{ t('gptAuto.simulator.target', { value: formatPercent(row.target_margin) }) }}</span>
                  </td>
                  <td class="px-3 py-2 tabular-nums">{{ row.min_required_multiplier !== undefined && row.min_required_multiplier !== null ? `${row.min_required_multiplier}×` : '—' }}</td>
                  <td class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ labels.outputSource(row.output_estimate_source) }}<template v-if="row.output_estimate_samples"> · n={{ row.output_estimate_samples }}</template>
                    <span v-if="row.below_target" class="block font-medium text-red-600 dark:text-red-400">{{ t('gptAuto.simulator.belowTarget') }}</span>
                    <span v-if="!row.pricing_available" class="block text-amber-600 dark:text-amber-400">{{ t('gptAuto.simulator.noPricing') }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.simulator.footnote') }}</p>
          </div>
        </div>
      </section>

      <!-- Save bar -->
      <div class="sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-3 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/95">
        <p v-if="hasErrors" class="mr-auto text-sm text-red-600 dark:text-red-400">{{ t('gptAuto.config.fixErrors') }}</p>
        <p v-else-if="dirty" class="mr-auto text-sm text-amber-600 dark:text-amber-400">{{ t('gptAuto.config.unsaved') }}</p>
        <button type="button" class="btn btn-secondary" :disabled="saving || !dirty" data-testid="gpt-auto-reset" @click="reset">{{ t('gptAuto.config.reset') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving || !dirty || hasErrors" data-testid="gpt-auto-save" @click="requestSave">
          {{ saving ? t('common.saving') : t('gptAuto.config.save') }}
        </button>
      </div>
    </template>

    <ConfirmDialog
      :show="confirmEnable"
      :title="t('gptAuto.config.confirmEnableTitle')"
      :message="t('gptAuto.config.confirmEnableMessage', { rate: form?.sample_rate ?? 0 })"
      :confirm-text="t('gptAuto.config.confirmEnable')"
      :cancel-text="t('common.cancel')"
      @confirm="save"
      @cancel="confirmEnable = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { gptAutoAPI, type GPTAutoConfig, type GPTAutoMeta, type GPTAutoModelProfile, type GPTAutoSimulationResult, type UpdateGPTAutoConfig } from '@/api/admin/gptAuto'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { cloneProfiles, formatPercent, formatUSD, parseIdList, parseStringList, toEditableConfig } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const emit = defineEmits<{ (e: 'saved', config: GPTAutoConfig): void }>()

const { t } = useI18n()
const appStore = useAppStore()
const labels = useGPTAutoLabels()

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const confirmEnable = ref(false)
const saved = ref<GPTAutoConfig | null>(null)
const form = ref<UpdateGPTAutoConfig | null>(null)
const meta = ref<GPTAutoMeta | null>(null)
const groupIdsText = ref('')
const modelFilterText = ref('')
const baseline = ref('')

const endpointOptions = computed(() => meta.value?.endpoints ?? ['chat_completions', 'responses'])
const effortLadder = computed(() => meta.value?.efforts ?? ['none', 'low', 'medium', 'high', 'xhigh', 'max'])
const nativeEfforts = (model: string) => meta.value?.models.find((entry) => entry.model === model)?.native_supported_efforts ?? effortLadder.value

const numericFields: { key: keyof UpdateGPTAutoConfig; label: string; hint?: string; min: number; max: number; step?: number }[] = [
  { key: 'classifier_timeout_ms', label: 'gptAuto.config.classifier.timeout', min: 200, max: 60000, step: 100 },
  { key: 'classifier_max_concurrency', label: 'gptAuto.config.classifier.concurrency', min: 1, max: 64 },
  { key: 'classifier_retry_count', label: 'gptAuto.config.classifier.retries', hint: 'gptAuto.config.classifier.retriesHint', min: 0, max: 3 },
  { key: 'classifier_daily_call_limit', label: 'gptAuto.config.classifier.dailyCalls', hint: 'gptAuto.config.classifier.zeroUnlimited', min: 0, max: 10000000 },
  { key: 'classifier_daily_cost_limit', label: 'gptAuto.config.classifier.dailyCost', hint: 'gptAuto.config.classifier.zeroUnlimited', min: 0, max: 1000000, step: 0.01 },
  { key: 'classification_max_context_chars', label: 'gptAuto.config.classifier.maxChars', hint: 'gptAuto.config.classifier.maxCharsHint', min: 100, max: 200000, step: 100 },
  { key: 'classification_context_messages', label: 'gptAuto.config.classifier.contextMessages', min: 0, max: 20 },
  { key: 'worker_count', label: 'gptAuto.config.classifier.workers', min: 1, max: 32 },
  { key: 'queue_size', label: 'gptAuto.config.classifier.queueSize', min: 1, max: 100000 },
  { key: 'decision_retention_days', label: 'gptAuto.config.classifier.retention', min: 1, max: 3650 },
]

const proxyIdInput = computed<number | string>({
  get: () => form.value?.classifier_proxy_id ?? '',
  set: (value) => {
    if (!form.value) return
    const id = Number(value)
    form.value.classifier_proxy_id = Number.isInteger(id) && id > 0 ? id : null
  },
})

function toPct(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return ''
  return String(Math.round(value * 10000) / 100)
}

function fromPct(event: Event): number | null {
  const raw = (event.target as HTMLInputElement).value.trim()
  if (raw === '') return null
  const value = Number(raw)
  return Number.isFinite(value) ? value / 100 : null
}

function buildPayload(): UpdateGPTAutoConfig | null {
  if (!form.value) return null
  return {
    ...form.value,
    group_ids: form.value.all_groups ? [] : parseIdList(groupIdsText.value),
    model_filter: parseStringList(modelFilterText.value),
    model_profiles: cloneProfiles(form.value.model_profiles),
  }
}

const dirty = computed(() => JSON.stringify(buildPayload()) !== baseline.value)

const profileErrors = computed(() => {
  const errors: Record<string, { multiplier?: boolean; weight?: boolean; efforts?: boolean; channel?: boolean }> = {}
  for (const profile of form.value?.model_profiles ?? []) {
    const entry = {
      multiplier: !(profile.billing_multiplier > 0 && profile.billing_multiplier <= 100),
      weight: !(Number.isInteger(profile.routing_weight) && profile.routing_weight >= 0 && profile.routing_weight <= 1000),
      efforts: profile.supported_efforts.length === 0,
      channel: (profile.upstream_costs ?? []).some((cost) => !cost.channel || cost.input_per_m < 0 || cost.output_per_m < 0 || cost.cached_input_per_m < 0),
    }
    if (entry.multiplier || entry.weight || entry.efforts || entry.channel) errors[profile.model] = entry
  }
  return errors
})

const hasErrors = computed(() => {
  const value = form.value
  if (!value) return true
  if (Object.keys(profileErrors.value).length > 0) return true
  if (!(value.sample_rate >= 0 && value.sample_rate <= 100)) return true
  if (!(value.global_min_margin >= 0 && value.global_min_margin < 1)) return true
  if (!(value.profit_safety_buffer >= 0 && value.profit_safety_buffer < 1)) return true
  if (value.endpoints.length === 0) return true
  if (!value.all_groups && parseIdList(groupIdsText.value).length === 0) return true
  return numericFields.some((field) => {
    const number = Number(value[field.key])
    return !Number.isFinite(number) || number < field.min || number > field.max
  })
})

function apply(config: GPTAutoConfig) {
  saved.value = config
  form.value = toEditableConfig(config)
  groupIdsText.value = (config.group_ids ?? []).join(', ')
  modelFilterText.value = (config.model_filter ?? []).join(', ')
  baseline.value = JSON.stringify(buildPayload())
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [config, metaResult] = await Promise.all([gptAutoAPI.getConfig(), gptAutoAPI.getMeta()])
    meta.value = metaResult
    apply(config)
  } catch (err) {
    loadError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function reset() {
  if (saved.value) apply(saved.value)
}

function requestSave() {
  if (!form.value || hasErrors.value) return
  if (form.value.shadow_enabled && !saved.value?.shadow_enabled) {
    confirmEnable.value = true
    return
  }
  void save()
}

async function save() {
  confirmEnable.value = false
  const payload = buildPayload()
  if (!payload) return
  saving.value = true
  try {
    const result = await gptAutoAPI.updateConfig(payload)
    apply(result)
    appStore.showSuccess(t('gptAuto.config.saved'))
    emit('saved', result)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('gptAuto.config.saveFailed')))
  } finally {
    saving.value = false
  }
}

function addChannel(profile: GPTAutoModelProfile) {
  if (!profile.upstream_costs) profile.upstream_costs = []
  profile.upstream_costs.push({ channel: '', enabled: true, input_per_m: 0, output_per_m: 0, cached_input_per_m: 0 })
}

// --- simulator ---
const simColumns = ['model', 'multiplier', 'revenue', 'upstream', 'classifier', 'profit', 'margin', 'required', 'basis']
const sim = reactive({ effort: 'medium', input_tokens: 2000, outputTokensText: '', include_classifier_cost: true })
const simLoading = ref(false)
const simError = ref('')
const simResult = ref<GPTAutoSimulationResult | null>(null)

async function runSimulation() {
  const payload = buildPayload()
  if (!payload) return
  simLoading.value = true
  simError.value = ''
  try {
    const output = sim.outputTokensText === '' ? undefined : Number(sim.outputTokensText)
    simResult.value = await gptAutoAPI.simulate({
      model_profiles: payload.model_profiles,
      global_min_margin: payload.global_min_margin,
      profit_safety_buffer: payload.profit_safety_buffer,
      effort: sim.effort,
      input_tokens: Number(sim.input_tokens) || 0,
      output_tokens: output !== undefined && Number.isFinite(output) ? output : undefined,
      include_classifier_cost: sim.include_classifier_cost,
      classifier_input_tokens: Math.min(Number(sim.input_tokens) || 0, Math.ceil((payload.classification_max_context_chars || 0) / 4)) + 400,
    })
  } catch (err) {
    simError.value = extractApiErrorMessage(err, t('gptAuto.simulator.failed'))
  } finally {
    simLoading.value = false
  }
}

onMounted(() => { void load() })

defineExpose({ load, dirty })
</script>
