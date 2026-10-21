<template>
  <div class="space-y-4" data-testid="gpt-auto-decisions">
    <div class="card">
      <div class="grid grid-cols-1 gap-3 border-b border-gray-100 p-4 dark:border-dark-700 md:grid-cols-3 xl:grid-cols-7">
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
                <td class="max-w-[10rem] truncate px-3 py-2 font-mono text-xs" :title="item.request_id">{{ item.request_id || '—' }}</td>
                <td class="px-3 py-2 font-mono text-xs">{{ item.actual_model || '—' }}<span v-if="item.actual_effort" class="block text-gray-500">{{ item.actual_effort }}</span></td>
                <td class="px-3 py-2 font-mono text-xs">{{ item.jev_model || '—' }}</td>
                <td class="px-3 py-2 font-mono text-xs" :class="item.final_model && item.final_model !== item.jev_model ? 'text-amber-600 dark:text-amber-400' : ''">{{ item.final_model || '—' }}</td>
                <td class="px-3 py-2">{{ labels.effort(item.jev_effort) }}</td>
                <td class="px-3 py-2" :class="item.final_effort && item.final_effort !== item.jev_effort ? 'text-amber-600 dark:text-amber-400' : ''">{{ labels.effort(item.final_effort) }}</td>
                <td class="px-3 py-2 tabular-nums">{{ item.complexity !== undefined && item.complexity !== null ? item.complexity.toFixed(2) : '—' }}</td>
                <td class="px-3 py-2 tabular-nums">{{ formatPercent(item.model_confidence, 0) }}</td>
                <td class="px-3 py-2 tabular-nums">{{ formatMs(item.classifier_latency_ms) }}</td>
                <td class="px-3 py-2 tabular-nums" :class="item.margin_below_target ? 'text-red-600 dark:text-red-400' : ''">
                  {{ formatPercent(item.est_gross_margin) }}
                  <span class="block text-[11px] text-gray-500">{{ formatUSD(item.est_revenue) }}</span>
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

    <BaseDialog :show="detailOpen" :title="t('gptAuto.detail.title', { id: detail?.id ?? '' })" width="extra-wide" @close="closeDetail">
      <div v-if="detailLoading" class="flex justify-center py-10"><div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div></div>
      <p v-else-if="detailError" role="alert" class="text-sm text-red-600">{{ detailError }}</p>
      <div v-else-if="detail" class="space-y-5 text-sm" data-testid="gpt-auto-detail">
        <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">{{ t('gptAuto.detail.notice') }}</div>

        <dl class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <div v-for="fact in detailFacts" :key="fact.label">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ fact.label }}</dt>
            <dd class="font-medium text-gray-900 dark:text-white">{{ fact.value }}</dd>
          </div>
        </dl>

        <div v-if="detail.evaluation_status === 'failed'" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-900/20 dark:text-red-300">
          {{ t('gptAuto.detail.failed', { error: labels.errorClass(detail.error_class), attempts: detail.attempts }) }}
        </div>

        <GPTAutoDecisionDetail v-if="detail.classification && detail.decision" :classification="detail.classification" :decision="detail.decision" show-actual :actual-usage="detail.actual_usage ?? null" />

        <section v-if="detail.evaluation_status === 'success'" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="gpt-auto-replay">
          <h3 class="font-semibold text-gray-900 dark:text-white">{{ t('gptAuto.replay.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('gptAuto.replay.hint') }}</p>
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GPTAutoDecisionDetail from './GPTAutoDecisionDetail.vue'
import { gptAutoAPI, type GPTAutoDecision, type GPTAutoReplayMode, type GPTAutoReplayResult } from '@/api/admin/gptAuto'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { formatMs, formatNumber, formatPercent, formatUSD } from '@/utils/gptAuto'
import { useGPTAutoLabels } from '@/composables/useGPTAutoLabels'

const { t } = useI18n()
const labels = useGPTAutoLabels()

const modelPool = ['gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-6-luna', 'gpt-6.1-sol']
const effortLadder = ['none', 'low', 'medium', 'high', 'xhigh', 'max']
const columns = ['time', 'requestId', 'actualModel', 'jevModel', 'finalModel', 'jevEffort', 'finalEffort', 'complexity', 'confidence', 'latency', 'margin', 'status', 'policy']
type RangeKey = '1h' | '24h' | '7d' | '30d'
const rangeOptions: RangeKey[] = ['1h', '24h', '7d', '30d']
const rangeHours: Record<RangeKey, number> = { '1h': 1, '24h': 24, '7d': 168, '30d': 720 }

const filters = reactive({
  range: '24h' as RangeKey,
  actual_model: '',
  final_model: '',
  final_effort: '',
  evaluation_status: '' as '' | 'success' | 'failed',
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
  if (item.evaluation_status === 'failed') return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (item.decision_status === 'adjusted') return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (item.decision_status === 'no_eligible_candidate') return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
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
    { label: t('gptAuto.detail.actual'), value: `${value.actual_model || '—'} / ${value.actual_effort || '—'}` },
    { label: t('gptAuto.detail.stream'), value: value.stream ? t('gptAuto.common.yes') : t('gptAuto.common.no') },
    { label: t('gptAuto.detail.tools'), value: value.has_tools ? t('gptAuto.common.yes') : t('gptAuto.common.no') },
    { label: t('gptAuto.detail.groupId'), value: value.group_id ?? '—' },
    { label: t('gptAuto.detail.classifier'), value: value.classifier_model || '—' },
    { label: t('gptAuto.detail.jevTokens'), value: `${formatNumber(value.classifier_input_tokens)} / ${formatNumber(value.classifier_output_tokens)}` },
    { label: t('gptAuto.detail.jevCost'), value: formatUSD(value.classifier_cost, 8) },
    { label: t('gptAuto.detail.latency'), value: `${formatMs(value.classifier_latency_ms)} (+${formatMs(value.queue_delay_ms)})` },
    { label: t('gptAuto.detail.policy'), value: `${value.policy_version} · ${value.policy_snapshot_hash.slice(0, 10) || '—'}` },
  ]
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

onMounted(() => { void load() })

defineExpose({ load })
</script>
