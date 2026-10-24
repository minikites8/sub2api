<template>
  <div class="space-y-6" data-testid="gpt-auto-live">
    <div v-if="loading" class="flex justify-center py-12">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>
    <p v-else-if="loadError" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ loadError }}</p>

    <template v-else-if="live">
      <div
        class="rounded-lg border p-3 text-sm"
        :class="savedEnabled ? 'border-amber-300 bg-amber-50 text-amber-900 dark:border-amber-700/60 dark:bg-amber-900/20 dark:text-amber-200' : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
        data-testid="gpt-auto-live-state"
      >
        <p class="font-medium">{{ savedEnabled ? t('gptAuto.live.stateOn') : t('gptAuto.live.stateOff') }}</p>
        <p class="mt-1 text-xs">{{ t('gptAuto.live.stateHint', { ttl: cacheTTL }) }}</p>
      </div>

      <!-- Rollout -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.live.rollout.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.rollout.hint') }}</p>
        </header>
        <div class="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-3">
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.rollout.enabled') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.rollout.enabledHint') }}</p>
            </div>
            <Toggle v-model="live.live_enabled" data-testid="gpt-auto-live-toggle" />
          </div>
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.rollout.internalOnly') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.rollout.internalOnlyHint') }}</p>
            </div>
            <Toggle v-model="live.internal_only" data-testid="gpt-auto-live-internal" />
          </div>
          <div>
            <span class="input-label">{{ t('gptAuto.live.rollout.endpoints') }}</span>
            <div class="mt-2 flex flex-wrap gap-4">
              <label v-for="endpoint in endpointOptions" :key="endpoint" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="live.endpoints" type="checkbox" :value="endpoint" class="rounded border-gray-300" />
                {{ labels.endpoint(endpoint) }}
              </label>
            </div>
          </div>
          <label>
            <span class="input-label">{{ t('gptAuto.live.rollout.apiKeys') }}</span>
            <input v-model="apiKeysText" type="text" class="input" placeholder="12, 34" data-testid="gpt-auto-live-keys" />
            <span class="input-hint">{{ t('gptAuto.live.rollout.apiKeysHint') }}</span>
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.rollout.groups') }}</span>
            <input v-model="groupsText" type="text" class="input" placeholder="1, 2" :disabled="live.internal_only" data-testid="gpt-auto-live-groups" />
            <span class="input-hint">{{ live.internal_only ? t('gptAuto.live.rollout.groupsDisabledHint') : t('gptAuto.live.rollout.groupsHint') }}</span>
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.rollout.budget') }}</span>
            <input v-model.number="live.live_request_budget" type="number" min="0" step="1" class="input" />
            <span class="input-hint">{{ t('gptAuto.live.rollout.budgetHint') }}</span>
          </label>
        </div>
      </section>

      <!-- Classifier and failure policy -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.live.classifier.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.classifier.hint') }}</p>
        </header>
        <div class="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-4">
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.timeout') }}</span>
            <input v-model.number="live.classifier_timeout_ms" type="number" min="200" max="30000" step="100" class="input" data-testid="gpt-auto-live-timeout" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.concurrency') }}</span>
            <input v-model.number="live.classifier_max_concurrency" type="number" min="1" max="256" step="1" class="input" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.retry') }}</span>
            <Select v-model="live.classifier_retry_count" :options="[{ value: 0, label: '0' }, { value: 1, label: '1' }]" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.failurePolicy') }}</span>
            <Select v-model="live.classifier_failure_policy" data-testid="gpt-auto-live-failure-policy" :options="[{ value: 'compatible_fallback', label: t('gptAuto.live.failurePolicies.compatible_fallback') }, { value: 'reject_request', label: t('gptAuto.live.failurePolicies.reject_request') }]" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.fallbackModel') }}</span>
            <Select v-model="live.fallback_model" :options="(poolModels).map((model) => ({ value: model, label: model }))" />
          </label>
          <label>
            <span class="input-label">{{ t('gptAuto.live.classifier.fallbackEffort') }}</span>
            <Select v-model="live.fallback_effort" :options="(efforts).map((effort) => ({ value: effort, label: labels.effort(effort) }))" />
          </label>
          <label class="xl:col-span-2">
            <span class="input-label">{{ t('gptAuto.live.classifier.clientEffort') }}</span>
            <Select v-model="live.client_effort_policy" :options="[{ value: 'jev', label: t('gptAuto.live.clientEffortPolicies.jev') }, { value: 'honor_client', label: t('gptAuto.live.clientEffortPolicies.honor_client') }]" />
          </label>
        </div>
      </section>

      <!-- Sessions, commercial -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.live.sessions.title') }}</h2>
        </header>
        <div class="grid grid-cols-1 gap-4 p-6 md:grid-cols-2 xl:grid-cols-3">
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.sessions.affinity') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.sessions.affinityHint') }}</p>
            </div>
            <Toggle v-model="live.session_affinity_enabled" />
          </div>
          <label>
            <span class="input-label">{{ t('gptAuto.live.sessions.ttl') }}</span>
            <input v-model.number="live.session_affinity_ttl_seconds" type="number" min="60" max="604800" step="60" class="input" />
          </label>
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.sessions.marginGuard') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.sessions.marginGuardHint') }}</p>
            </div>
            <Toggle v-model="live.margin_guard_enabled" />
          </div>
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.sessions.billingMultiplier') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.sessions.billingMultiplierHint') }}</p>
            </div>
            <Toggle v-model="live.apply_billing_multiplier" data-testid="gpt-auto-live-billing" />
          </div>
          <div class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptAuto.live.sessions.customerExport') }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.sessions.customerExportHint') }}</p>
            </div>
            <Toggle v-model="live.customer_export_enabled" />
          </div>
        </div>
      </section>

      <!-- Capabilities -->
      <section class="card">
        <header class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.live.capabilities.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.live.capabilities.hint') }}</p>
        </header>
        <div class="overflow-x-auto p-6">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="text-left text-xs text-gray-500">
                <th class="py-2 pr-4">{{ t('gptAuto.live.capabilities.model') }}</th>
                <th class="py-2 pr-4">{{ t('gptAuto.live.capabilities.vision') }}</th>
                <th class="py-2 pr-4">{{ t('gptAuto.live.capabilities.structured') }}</th>
                <th class="py-2 pr-4">{{ t('gptAuto.live.capabilities.stateful') }}</th>
                <th class="py-2 pr-4">{{ t('gptAuto.live.capabilities.maxContext') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in capabilities" :key="row.model" :data-testid="`gpt-auto-live-cap-${row.model}`">
                <td class="py-2 pr-4 font-mono text-xs">{{ row.model }}</td>
                <td class="py-2 pr-4"><input v-model="row.caps.vision" type="checkbox" class="rounded border-gray-300" /></td>
                <td class="py-2 pr-4"><input v-model="row.caps.structured_outputs" type="checkbox" class="rounded border-gray-300" /></td>
                <td class="py-2 pr-4"><input v-model="row.caps.stateful_responses" type="checkbox" class="rounded border-gray-300" /></td>
                <td class="py-2 pr-4"><input v-model.number="row.caps.max_context_tokens" type="number" min="0" class="input w-32" /></td>
              </tr>
            </tbody>
          </table>
          <p v-if="retired.length" class="mt-3 text-xs text-gray-500" data-testid="gpt-auto-live-retired">{{ t('gptAuto.live.capabilities.retired', { models: retired.join(', ') }) }}</p>
        </div>
      </section>

      <div class="flex flex-wrap items-center justify-end gap-3">
        <p v-if="saveError" role="alert" class="text-sm text-red-600">{{ saveError }}</p>
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="load">{{ t('gptAuto.config.reset') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving" data-testid="gpt-auto-live-save" @click="requestSave">{{ saving ? t('gptAuto.common.working') : t('common.save') }}</button>
      </div>

      <!-- Runtime and shadow vs live comparison -->
      <section class="card" data-testid="gpt-auto-live-runtime">
        <header class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.live.runtime.title') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="compareLoading" @click="loadComparison">{{ t('gptAuto.live.runtime.refresh') }}</button>
        </header>
        <div class="space-y-4 p-6 text-sm">
          <dl v-if="runtime" class="grid grid-cols-2 gap-3 md:grid-cols-4">
            <div><dt class="text-xs text-gray-500">{{ t('gptAuto.live.runtime.circuit') }}</dt><dd class="font-medium">{{ runtime.circuit_state }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('gptAuto.live.runtime.active') }}</dt><dd class="font-medium">{{ runtime.active_calls }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('gptAuto.live.runtime.today') }}</dt><dd class="font-medium">{{ runtime.today_live_requests ?? '—' }} / {{ runtime.daily_budget || '∞' }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('gptAuto.live.runtime.persistQueue') }}</dt><dd class="font-medium">{{ runtime.persist_queue_length }}</dd></div>
          </dl>
          <p class="text-xs text-gray-500">{{ t('gptAuto.live.runtime.nodeNote') }}</p>
          <p v-if="compareError" role="alert" class="text-sm text-red-600">{{ compareError }}</p>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div v-for="side in comparison" :key="side.mode" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" :data-testid="`gpt-auto-live-compare-${side.mode}`">
              <p class="font-medium text-gray-900 dark:text-white">{{ t(`gptAuto.live.modes.${side.mode}`) }} · {{ side.total }}</p>
              <p class="text-xs text-gray-500">{{ side.mode === 'shadow' ? t('gptAuto.live.runtime.shadowNote') : t('gptAuto.live.runtime.liveNote') }}</p>
              <ul class="mt-2 space-y-1">
                <li v-for="bucket in side.models" :key="bucket.key" class="flex justify-between font-mono text-xs"><span>{{ bucket.key || '—' }}</span><span>{{ bucket.count }}</span></li>
                <li v-if="side.models.length === 0" class="text-xs text-gray-500">{{ t('gptAuto.common.noData') }}</li>
              </ul>
            </div>
          </div>
        </div>
      </section>
    </template>

    <ConfirmDialog
      :show="confirmOpen"
      :title="t('gptAuto.live.confirm.title')"
      :message="t('gptAuto.live.confirm.message')"
      :confirm-text="t('gptAuto.live.confirm.confirm')"
      danger
      @confirm="save"
      @cancel="confirmOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
import Select from '@/components/common/Select.vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import {
  gptAutoAPI,
  type GPTAutoCount,
  type GPTAutoLiveConfig,
  type GPTAutoLiveRuntimeStatus,
  type GPTAutoModelCapabilities,
  type GPTAutoMode,
} from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { parseIdList, toEditableConfig } from '@/utils/gptAuto'
import { useAppStore } from '@/stores/app'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const { t } = useI18n()
const labels = useGPTAutoLabels()
const appStore = useAppStore()

const endpointOptions = ['chat_completions', 'responses'] as const
const efforts = ['none', 'low', 'medium', 'high', 'xhigh', 'max']

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const saveError = ref('')
const confirmOpen = ref(false)
const live = ref<GPTAutoLiveConfig | null>(null)
const savedEnabled = ref(false)
const apiKeysText = ref('')
const groupsText = ref('')
const poolModels = ref<string[]>([])
const retired = ref<string[]>([])
const capabilities = ref<{ model: string; caps: GPTAutoModelCapabilities }[]>([])
const cacheTTL = ref(30)

const defaultCaps = (): GPTAutoModelCapabilities => ({ vision: true, structured_outputs: true, stateful_responses: true, max_context_tokens: 0 })

async function load() {
  loading.value = true
  loadError.value = ''
  saveError.value = ''
  try {
    const [config, meta] = await Promise.all([gptAutoAPI.getConfig(), gptAutoAPI.getMeta()])
    if (!config.live) throw new Error('missing live config')
    live.value = { ...config.live, endpoints: [...(config.live.endpoints ?? [])] }
    savedEnabled.value = config.live.live_enabled
    apiKeysText.value = (config.live.allowed_api_keys ?? []).join(', ')
    groupsText.value = (config.live.allowed_groups ?? []).join(', ')
    poolModels.value = (config.model_profiles ?? []).map((profile) => profile.model)
    retired.value = (config.retired_model_profiles ?? []).map((profile) => profile.model)
    capabilities.value = (config.model_profiles ?? []).map((profile) => ({ model: profile.model, caps: { ...(profile.capabilities ?? defaultCaps()) } }))
    cacheTTL.value = meta.config_cache_ttl_seconds ?? 30
  } catch (err) {
    loadError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    loading.value = false
  }
}

function requestSave() {
  // Turning live routing on changes customer traffic: confirm explicitly.
  if (live.value?.live_enabled && !savedEnabled.value) {
    confirmOpen.value = true
    return
  }
  void save()
}

async function save() {
  confirmOpen.value = false
  if (!live.value) return
  saving.value = true
  saveError.value = ''
  try {
    // Re-read so a save here never overwrites concurrent edits to the other
    // configuration sections.
    const latest = toEditableConfig(await gptAutoAPI.getConfig())
    latest.live = {
      ...live.value,
      allowed_api_keys: parseIdList(apiKeysText.value),
      allowed_groups: parseIdList(groupsText.value),
    }
    latest.model_profiles = latest.model_profiles.map((profile) => {
      const row = capabilities.value.find((item) => item.model === profile.model)
      return row ? { ...profile, capabilities: { ...row.caps, max_context_tokens: Math.max(0, Math.floor(Number(row.caps.max_context_tokens) || 0)) } } : profile
    })
    await gptAutoAPI.updateConfig(latest)
    appStore.showSuccess(t('gptAuto.live.saved'))
    await load()
  } catch (err) {
    saveError.value = extractApiErrorMessage(err, t('gptAuto.live.saveFailed'))
  } finally {
    saving.value = false
  }
}

// --- runtime and shadow vs live comparison ---------------------------------
const runtime = ref<GPTAutoLiveRuntimeStatus | null>(null)
const compareLoading = ref(false)
const compareError = ref('')
const comparison = ref<{ mode: GPTAutoMode; total: number; models: GPTAutoCount[] }[]>([])

const comparisonWindow = computed(() => {
  const end = new Date()
  return { start_time: new Date(end.getTime() - 24 * 3600 * 1000).toISOString(), end_time: end.toISOString() }
})

async function loadComparison() {
  compareLoading.value = true
  compareError.value = ''
  try {
    const modes: GPTAutoMode[] = ['shadow', 'live']
    const results = await Promise.all(modes.map((mode) => gptAutoAPI.getOverview({ ...comparisonWindow.value, mode })))
    runtime.value = results[1].runtime?.live ?? null
    comparison.value = results.map((overview, index) => ({
      mode: modes[index],
      total: overview.stored?.total ?? 0,
      models: overview.stored?.final_models ?? [],
    }))
  } catch (err) {
    compareError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    compareLoading.value = false
  }
}

onMounted(() => {
  void load()
  void loadComparison()
})
</script>
