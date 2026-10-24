<template>
  <div class="space-y-4" data-testid="gpt-auto-decisions">
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.logs.modeHint') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="gpt-auto-export-open" @click="exportOpen = true">
          <Icon name="download" size="sm" /> {{ t('gptAuto.export.open') }}
        </button>
      </div>
      <div class="grid grid-cols-1 gap-3 border-b border-gray-100 p-4 dark:border-dark-700 md:grid-cols-4 xl:grid-cols-8">
        <label>
          <span class="input-label">{{ t('gptAuto.export.mode') }}</span>
          <select v-model="filters.mode" class="input" data-testid="gpt-auto-logs-mode" @change="reload">
            <option value="">{{ t('gptAuto.export.modeAll') }}</option>
            <option value="shadow">{{ t('gptAuto.live.modes.shadow') }}</option>
            <option value="live">{{ t('gptAuto.live.modes.live') }}</option>
          </select>
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.range') }}</span>
          <select v-model="filters.range" class="input" @change="reload">
            <option v-for="option in rangeOptions" :key="option" :value="option">{{ t(`gptAuto.overview.ranges.${option}`) }}</option>
          </select>
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.actualModel') }}</span>
          <input v-model.trim="filters.actual_model" type="text" class="input" maxlength="128" @keyup.enter="reload" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.finalModel') }}</span>
          <select v-model="filters.final_model" class="input" @change="reload">
            <option value="">{{ t('gptAuto.common.all') }}</option>
            <option v-for="model in modelPool" :key="model" :value="model">{{ model }}</option>
          </select>
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.effort') }}</span>
          <select v-model="filters.final_effort" class="input" @change="reload">
            <option value="">{{ t('gptAuto.common.all') }}</option>
            <option v-for="effort in effortLadder" :key="effort" :value="effort">{{ labels.effort(effort) }}</option>
          </select>
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.status') }}</span>
          <select v-model="filters.evaluation_status" class="input" @change="reload">
            <option value="">{{ t('gptAuto.common.all') }}</option>
            <option value="success">{{ labels.evaluationStatus('success') }}</option>
            <option value="failed">{{ labels.evaluationStatus('failed') }}</option>
            <option value="skipped">{{ labels.evaluationStatus('skipped') }}</option>
          </select>
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.groupId') }}</span>
          <input v-model="filters.group_id" type="number" min="1" class="input" @keyup.enter="reload" />
        </label>
        <label>
          <span class="input-label">{{ t('gptAuto.logs.filters.requestId') }}</span>
          <div class="flex gap-2">
            <input v-model.trim="filters.request_id" type="text" class="input" maxlength="128" @keyup.enter="reload" />
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="reload"><Icon name="search" size="sm" /></button>
          </div>
        </label>
      </div>

      <p v-if="error" role="alert" class="m-4 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>

      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th v-for="column in columns" :key="column" class="whitespace-nowrap px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t(`gptAuto.logs.columns.${column}`) }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-if="loading"><td :colspan="columns.length" class="px-3 py-8 text-center text-gray-500">{{ t('common.loading') }}</td></tr>
            <tr v-else-if="items.length === 0"><td :colspan="columns.length" class="px-3 py-8 text-center text-gray-500" data-testid="gpt-auto-decisions-empty">{{ t('gptAuto.logs.empty') }}</td></tr>
            <template v-else>
              <tr v-for="item in items" :key="item.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/60" :data-testid="`gpt-auto-decision-${item.id}`" @click="openDetail(item.id)">
                <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">{{ formatDateTime(item.created_at) }}</td>
                <td class="px-3 py-2" :data-testid="`gpt-auto-mode-${item.id}`">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="item.mode === 'live' ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">{{ t(`gptAuto.live.modes.${item.mode === 'live' ? 'live' : 'shadow'}`) }}</span>
                  <span v-if="item.features?.switching" class="block text-[11px] text-primary-600 dark:text-primary-400">{{ item.features.switching.affinity_type }} · {{ item.features.switching.switch_direction }}<span v-if="item.features.switching.effort_changed"> · {{ t('gptAuto.dynamic.effortChanged') }}</span></span>
                  <span v-if="item.mode === 'live' && item.request_status" class="block text-[11px] text-gray-500">{{ t(`gptAuto.live.requestStatuses.${item.request_status}`) }}</span>
                </td>
                <td class="max-w-[10rem] truncate px-3 py-2 font-mono text-xs" :title="item.request_id">{{ item.request_id || '—' }}</td>
                <td class="px-3 py-2 font-mono text-xs">{{ item.actual_model || '—' }}<span v-if="item.actual_effort" class="block text-gray-500">{{ item.actual_effort }}</span></td>
                <td class="px-3 py-2 font-mono text-xs">{{ item.jev_model || '—' }}</td>
                <td class="px-3 py-2 font-mono text-xs" :class="item.final_model && item.final_model !== item.jev_model ? 'text-amber-600 dark:text-amber-400' : ''">{{ item.final_model || '—' }}</td>
                <td class="px-3 py-2">{{ labels.effort(item.jev_effort) }}</td>
                <td class="px-3 py-2" :class="item.final_effort && item.final_effort !== item.jev_effort ? 'text-amber-600 dark:text-amber-400' : ''">{{ labels.effort(item.final_effort) }}</td>
                <td class="px-3 py-2 tabular-nums">{{ item.complexity !== undefined && item.complexity !== null ? item.complexity.toFixed(2) : '—' }}</td>
                <td class="px-3 py-2 tabular-nums">{{ formatPercent(item.model_confidence, 0) }}</td>
                <td class="px-3 py-2 tabular-nums">{{ formatMs(item.classifier_latency_ms) }}</td>
                <td class="px-3 py-2 tabular-nums" :data-testid="`gpt-auto-margin-${item.id}`">
                  <template v-if="rowMargins(item)">
                    <span :class="marginStatusClass(rowMargins(item)!.model)">{{ formatPercent(item.est_model_gross_margin) }}</span>
                    <span class="text-gray-400"> / </span>
                    <span :class="marginStatusClass(rowMargins(item)!.full)">{{ formatPercent(item.est_gross_margin) }}</span>
                    <span class="block text-[11px] text-gray-500">{{ labels.estimateQuality(item.estimate_quality || 'legacy') }}</span>
                  </template>
                  <span v-else class="text-gray-500">N/A</span>
                </td>
                <td class="px-3 py-2">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass(item)">
                    {{ item.evaluation_status === 'failed' ? labels.errorClass(item.error_class) : labels.decisionStatus(item.decision_status) }}
                  </span>
                </td>
                <td class="px-3 py-2 font-mono text-xs text-gray-500">{{ item.policy_version }}</td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
      <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="onPage" @update:pageSize="onPageSize" />
    </div>

    <BaseDialog :show="detailOpen" :title="t(detail?.mode === 'live' ? 'gptAuto.dynamic.liveTitle' : 'gptAuto.detail.title', { id: detail?.id ?? '' })" width="extra-wide" @close="closeDetail">
      <div v-if="detailLoading" class="flex justify-center py-10"><div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div></div>
      <p v-else-if="detailError" role="alert" class="text-sm text-red-600">{{ detailError }}</p>
      <div v-else-if="detail" class="space-y-5 text-sm" data-testid="gpt-auto-detail">
        <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">{{ t(detail.mode === 'live' ? 'gptAuto.dynamic.liveNotice' : 'gptAuto.detail.notice') }}</div>

        <dl class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <div v-for="fact in detailFacts" :key="fact.label">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ fact.label }}</dt>
            <dd class="font-medium text-gray-900 dark:text-white">{{ fact.value }}</dd>
          </div>
        </dl>

        <section v-if="detail.features.switching" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="dynamic-decision-detail">
          <dl class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div><dt>{{ t('gptAuto.dynamic.previous') }}</dt><dd>{{ detail.features.switching.previous_model || '—' }} / {{ detail.features.switching.previous_effort || '—' }}</dd></div>
            <div><dt>{{ t('gptAuto.dynamic.jev') }}</dt><dd>{{ detail.jev_model || '—' }} / {{ detail.jev_effort || '—' }}</dd></div>
            <div><dt>{{ t('gptAuto.dynamic.policy') }}</dt><dd>{{ detail.features.switching.policy_model || '—' }} / {{ detail.features.switching.policy_effort || '—' }}</dd></div>
            <div><dt>{{ t('gptAuto.dynamic.executed') }}</dt><dd>{{ detail.final_model || '—' }} / {{ detail.final_effort || '—' }}</dd></div>
            <div><dt>{{ t('gptAuto.dynamic.reason') }}</dt><dd>{{ detail.features.switching.switch_direction }} · {{ detail.features.switching.switch_block_reason || detail.features.switching.switch_reason }}</dd></div>
            <div><dt>{{ t('gptAuto.dynamic.affinity') }}</dt><dd>{{ detail.features.switching.affinity_type }} · {{ detail.features.switching.state_dependency }}</dd></div>
          </dl>
          <p v-if="detail.features.switching.model_switched" class="mt-2 text-primary-600">{{ t('gptAuto.dynamic.modelSwitched') }}</p>
          <p v-if="detail.features.switching.effort_changed" class="mt-2 text-primary-600">{{ t('gptAuto.dynamic.effortChanged') }}</p>
          <p v-if="detail.evaluation_status === 'skipped' && detail.features.switching.affinity_type === 'hard'" class="mt-2">{{ t('gptAuto.dynamic.skipped') }}</p>
          <p class="mt-2 font-mono text-xs">{{ detail.features.switching.classification_trigger }}</p>
          <details v-if="detail.features.switching.history?.length" class="mt-3">
            <summary>{{ t('gptAuto.dynamic.history') }}</summary>
            <ol class="mt-2 space-y-1 font-mono text-xs"><li v-for="(turn, index) in detail.features.switching.history" :key="index">#{{ turn.sequence }} · {{ turn.model }} / {{ turn.effort }} · {{ turn.direction }}</li></ol>
          </details>
        </section>
        <p v-else-if="detail.mode === 'live' && detail.routing_reason === 'session_affinity'">{{ t('gptAuto.dynamic.skippedLegacy') }}</p>

        <GPTAutoContextDiagnostics
          :context="detail.features.context ?? null"
          :classification="detail.classification ?? null"
          :decision="detail.decision ?? null"
          :classifier-hash="detail.classifier_config_hash ?? ''"
          :classifier-schema="detail.classifier_config?.snapshot.schema_version ?? null"
          :runtime="runtime"
        />

        <div v-if="detail.evaluation_status === 'failed'" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-900/20 dark:text-red-300">
          {{ t('gptAuto.detail.failed', { error: labels.errorClass(detail.error_class), attempts: detail.attempts }) }}
        </div>

        <GPTAutoDecisionDetail v-if="detail.classification && detail.decision" :classification="detail.classification" :decision="detail.decision" show-actual :actual-usage="detail.actual_usage ?? null" :observed="detail.observed_economics ?? null" :actual-input-economics="detail.actual_input_economics ?? null" />

        <details v-if="detail.evaluation_status === 'success'" class="rounded-lg border border-gray-200 p-3 text-xs dark:border-dark-600" data-testid="gpt-auto-detail-classifier">
          <summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300">{{ t('gptAuto.detail.classifierConfig') }} · {{ detail.classifier_config_hash ? detail.classifier_config_hash.slice(0, 10) : t('gptAuto.detail.classifierLegacy') }}</summary>
          <p v-if="!detail.classifier_config_hash" class="mt-2 text-gray-500">{{ t('gptAuto.detail.classifierLegacyHint') }}</p>
          <p v-else-if="!detail.classifier_config" class="mt-2 text-gray-500">{{ t('gptAuto.detail.classifierMissing') }}</p>
          <div v-else class="mt-2 space-y-2">
            <p class="text-gray-500">{{ t('gptAuto.classifier.revision', { revision: detail.classifier_config.revision }) }} · {{ detail.classifier_config.snapshot.schema_version }} · {{ detail.classifier_config.snapshot.state_version }}</p>
            <div v-for="(question, id) in detail.classifier_config.snapshot.questions" :key="id">
              <p class="font-mono font-medium text-gray-700 dark:text-gray-300">{{ id }}</p>
              <p class="whitespace-pre-wrap text-gray-600 dark:text-gray-400">{{ question.instructions }}</p>
            </div>
          </div>
        </details>

        <section v-if="detail.evaluation_status === 'success'" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="gpt-auto-replay">
          <h3 class="font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.replay.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.replay.hint') }}</p>
          <p class="mt-1 text-xs text-amber-700 dark:text-amber-300" data-testid="gpt-auto-replay-instructions-note">{{ t('gptAuto.replay.instructionsNote') }}</p>
          <div class="mt-3 flex flex-wrap items-center gap-3">
            <select v-model="replayMode" class="input w-auto">
              <option value="historical">{{ t('gptAuto.replay.modes.historical') }}</option>
              <option value="current">{{ t('gptAuto.replay.modes.current') }}</option>
            </select>
            <button type="button" class="btn btn-secondary" :disabled="replayLoading" data-testid="gpt-auto-replay-run" @click="runReplay">{{ replayLoading ? t('gptAuto.common.working') : t('gptAuto.replay.run') }}</button>
          </div>
          <p v-if="replayError" role="alert" class="mt-2 text-sm text-red-600">{{ replayError }}</p>
          <div v-if="replayResult" class="mt-3 space-y-2">
            <p :class="replayResult.changed ? 'text-amber-600 dark:text-amber-400' : 'text-emerald-600 dark:text-emerald-400'">
              {{ replayResult.changed ? t('gptAuto.replay.changed') : t('gptAuto.replay.unchanged') }}
              — {{ replayResult.replayed.final_model || '—' }} / {{ labels.effort(replayResult.replayed.final_effort) }}
              ({{ replayResult.replayed.policy_version }})
            </p>
            <GPTAutoDecisionDetail :classification="detail.classification!" :decision="replayResult.replayed" compact />
          </div>
        </section>
      </div>
    </BaseDialog>

    <GPTAutoExportDialog :show="exportOpen" :preset="exportPreset" @close="exportOpen = false" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GPTAutoDecisionDetail from './GPTAutoDecisionDetail.vue'
import GPTAutoContextDiagnostics from './GPTAutoContextDiagnostics.vue'
import GPTAutoExportDialog from './GPTAutoExportDialog.vue'
import { gptAutoAPI, type GPTAutoDecision, type GPTAutoExportPreset, type GPTAutoMeta, type GPTAutoMode, type GPTAutoReplayMode, type GPTAutoReplayResult } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { formatMs, formatNumber, formatPercent, formatUSD, marginStatusClass, type MarginStatus } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const { t } = useI18n()
const labels = useGPTAutoLabels()

const modelPool = ['gpt-5.6-luna', 'gpt-6-luna', 'gpt-6.1-sol']
const effortLadder = ['none', 'low', 'medium', 'high', 'xhigh', 'max']
const columns = ['time', 'mode', 'requestId', 'actualModel', 'jevModel', 'finalModel', 'jevEffort', 'finalEffort', 'complexity', 'confidence', 'latency', 'margin', 'status', 'policy']
type RangeKey = '1h' | '24h' | '7d' | '30d'
const rangeOptions: RangeKey[] = ['1h', '24h', '7d', '30d']
const rangeHours: Record<RangeKey, number> = { '1h': 1, '24h': 24, '7d': 168, '30d': 720 }

const filters = reactive({
  mode: '' as GPTAutoMode | '',
  range: '24h' as RangeKey,
  actual_model: '',
  final_model: '',
  final_effort: '',
  evaluation_status: '' as '' | 'success' | 'failed' | 'skipped',
  group_id: '' as string | number,
  request_id: '',
})
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const items = ref<GPTAutoDecision[]>([])
const loading = ref(false)
const error = ref('')
let sequence = 0

async function load() {
  const current = ++sequence
  loading.value = true
  error.value = ''
  const end = new Date()
  const start = new Date(end.getTime() - rangeHours[filters.range] * 3600 * 1000)
  const groupId = Number(filters.group_id)
  try {
    const result = await gptAutoAPI.listDecisions({
      page: pagination.page,
      page_size: pagination.page_size,
      start_time: start.toISOString(),
      end_time: end.toISOString(),
      actual_model: filters.actual_model,
      final_model: filters.final_model,
      final_effort: filters.final_effort,
      evaluation_status: filters.evaluation_status,
      group_id: Number.isInteger(groupId) && groupId > 0 ? groupId : undefined,
      request_id: filters.request_id,
      mode: filters.mode,
    })
    if (current !== sequence) return
    items.value = result.items ?? []
    pagination.total = result.total
  } catch (err) {
    if (current === sequence) error.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    if (current === sequence) loading.value = false
  }
}

function reload() {
  pagination.page = 1
  void load()
}
function onPage(page: number) {
  pagination.page = page
  void load()
}
function onPageSize(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void load()
}

function statusClass(item: GPTAutoDecision) {
  if (item.evaluation_status === 'failed' || item.request_status === 'failed' || item.request_status === 'rejected') return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (item.decision_status === 'adjusted') return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (item.decision_status === 'no_eligible_candidate') return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
}

// Margin colours for a list row: model-only / fully loaded. Rows without a
// model margin predate the split; their status is derived from the flag.
function rowMargins(item: GPTAutoDecision): { model: MarginStatus; full: MarginStatus } | null {
  if (item.est_gross_margin === undefined || item.est_gross_margin === null) return null
  const full: MarginStatus = item.est_gross_margin < 0 ? 'loss' : item.margin_below_target ? 'below_target' : 'meets_target'
  const model: MarginStatus = item.est_model_gross_margin === undefined || item.est_model_gross_margin === null
    ? 'unknown'
    : item.est_model_gross_margin < 0 ? 'loss' : item.model_margin_below_target ? 'below_target' : 'meets_target'
  return { model, full }
}
// --- detail ---
const detailOpen = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const detail = ref<GPTAutoDecision | null>(null)
const replayMode = ref<GPTAutoReplayMode>('current')
const replayLoading = ref(false)
const replayError = ref('')
const replayResult = ref<GPTAutoReplayResult | null>(null)

async function openDetail(id: number) {
  detailOpen.value = true
  detailLoading.value = true
  detailError.value = ''
  detail.value = null
  replayResult.value = null
  replayError.value = ''
  try {
    detail.value = await gptAutoAPI.getDecision(id)
  } catch (err) {
    detailError.value = extractApiErrorMessage(err, t('gptAuto.common.loadFailed'))
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  detailOpen.value = false
}

const detailFacts = computed(() => {
  const value = detail.value
  if (!value) return []
  return [
    { label: t('gptAuto.logs.columns.time'), value: formatDateTime(value.created_at) },
    { label: t('gptAuto.logs.columns.requestId'), value: value.request_id || '—' },
    { label: t('gptAuto.detail.endpoint'), value: labels.endpoint(value.endpoint) },
    { label: t(value.mode === 'live' ? 'gptAuto.dynamic.executed' : 'gptAuto.detail.actual'), value: value.mode === 'live' ? `${value.final_model || '—'} / ${value.final_effort || '—'}` : `${value.actual_model || '—'} / ${value.actual_effort || '—'}` },
    { label: t('gptAuto.detail.stream'), value: value.stream ? t('gptAuto.common.yes') : t('gptAuto.common.no') },
    { label: t('gptAuto.detail.tools'), value: value.has_tools ? t('gptAuto.common.yes') : t('gptAuto.common.no') },
    { label: t('gptAuto.detail.groupId'), value: value.group_id ?? '—' },
    { label: t('gptAuto.detail.classifier'), value: value.classifier_model || '—' },
    { label: t('gptAuto.detail.jevTokens'), value: `${formatNumber(value.classifier_input_tokens)} / ${formatNumber(value.classifier_output_tokens)}` },
    { label: t('gptAuto.detail.jevCost'), value: formatUSD(value.classifier_cost) },
    { label: t('gptAuto.detail.latency'), value: `${formatMs(value.classifier_latency_ms)} (+${formatMs(value.queue_delay_ms)})` },
    { label: t('gptAuto.detail.policy'), value: `${value.policy_version} · ${value.policy_snapshot_hash.slice(0, 10) || '—'}` },
    { label: t('gptAuto.detail.classifierConfig'), value: value.classifier_config_hash ? value.classifier_config_hash.slice(0, 10) : t('gptAuto.detail.classifierLegacy') },
    ...(value.mode === 'live'
      ? [
          { label: t('gptAuto.live.detail.routingReason'), value: value.routing_reason || '—' },
          { label: t('gptAuto.live.detail.fallbackReason'), value: value.fallback_reason || '—' },
          { label: t('gptAuto.live.detail.affinity'), value: value.affinity_outcome || '—' },
          { label: t('gptAuto.live.detail.status'), value: `${value.request_status || '—'} · ${value.http_status ?? '—'}${value.error_category ? ` · ${value.error_category}` : ''}` },
          { label: t('gptAuto.live.detail.servedModel'), value: value.served_model || '—' },
          { label: t('gptAuto.live.detail.account'), value: value.upstream_account_id ?? '—' },
          { label: t('gptAuto.live.detail.multiplier'), value: value.billing_multiplier ?? '—' },
          { label: t('gptAuto.pricing.active'), value: value.pricing_mode ? `${value.pricing_model} · ${t(`gptAuto.pricing.modes.${value.pricing_mode}`)} · v${value.pricing_config_version}` : '—' },
          { label: t('gptAuto.live.detail.latency'), value: `${formatMs(value.gateway_latency_ms ?? null)} / ${formatMs(value.upstream_ttft_ms ?? null)} / ${formatMs(value.total_latency_ms ?? null)}` },
          { label: t('gptAuto.live.detail.retries'), value: value.retry_count ?? 0 },
        ]
      : []),
  ]
})

// --- export -----------------------------------------------------------------
const exportOpen = ref(false)
const exportPreset = computed<GPTAutoExportPreset>(() => {
  const end = new Date()
  const groupId = Number(filters.group_id)
  return {
    start: new Date(end.getTime() - rangeHours[filters.range] * 3600 * 1000),
    end,
    mode: filters.mode,
    final_model: filters.final_model,
    final_effort: filters.final_effort,
    evaluation_status: filters.evaluation_status,
    group_id: Number.isInteger(groupId) && groupId > 0 ? groupId : undefined,
  }
})

async function runReplay() {
  if (!detail.value) return
  replayLoading.value = true
  replayError.value = ''
  try {
    replayResult.value = await gptAutoAPI.replayDecision(detail.value.id, { mode: replayMode.value })
  } catch (err) {
    replayError.value = extractApiErrorMessage(err, t('gptAuto.replay.failed'))
  } finally {
    replayLoading.value = false
  }
}

// Versions compiled into the backend node serving this page. Each decision
// carries its own versions; these are only shown when they differ.
const runtime = ref<Pick<GPTAutoMeta, 'state_version' | 'classifier_schema' | 'diagnostics_version'> | null>(null)
async function loadRuntime() {
  try {
    const meta = await gptAutoAPI.getMeta()
    runtime.value = { state_version: meta.state_version, classifier_schema: meta.classifier_schema, diagnostics_version: meta.diagnostics_version }
  } catch {
    runtime.value = null
  }
}

onMounted(() => {
  void load()
  void loadRuntime()
})

defineExpose({ load })
</script>
