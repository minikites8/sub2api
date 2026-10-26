<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5 p-4 sm:p-6">
      <h1 class="text-2xl font-semibold">{{ t('gptQos.title') }}</h1>
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('gptQos.defaultsOff') }}</p>
      <p v-if="error" role="alert" class="rounded bg-red-50 p-3 text-red-700">{{ error }}</p>
      <form v-if="config" class="space-y-5" @submit.prevent="save">
        <label class="block"><span class="input-label">{{ t('gptQos.rolloutGroups') }}</span><input v-model="rolloutGroups" class="input" pattern="[0-9, ]*" /></label>
        <fieldset class="grid gap-4 rounded-lg border p-4 sm:grid-cols-2 dark:border-dark-700">
          <label v-for="flag in flags" :key="flag.key" class="flex items-center gap-2"><input v-model="config[flag.key]" type="checkbox" />{{ t(`gptQos.${flag.label}`) }}</label>
          <label class="flex items-center gap-2"><input v-model="config.peak.enabled" type="checkbox" />{{ t('gptQos.peakEnabled') }}</label>
          <label class="flex items-center gap-2"><input v-model="config.peak.strict_reserve" type="checkbox" />{{ t('gptQos.strictReserve') }}</label>
          <label v-for="field in numberFields" :key="field.key"><span class="input-label">{{ t(`gptQos.${field.label}`) }}</span><input v-model.number="config[field.key]" class="input" type="number" :min="field.min" :max="field.max" :step="'step' in field ? field.step : 1" required /></label>
          <label v-for="field in peakFields" :key="field.key"><span class="input-label">{{ t(`gptQos.${field.label}`) }}</span><input v-model.number="config.peak[field.key]" class="input" type="number" :min="field.min" :max="field.max" :step="field.step" required /></label>
        </fieldset>
        <fieldset class="space-y-3 rounded-lg border p-4 dark:border-dark-700">
          <legend class="px-1 font-medium">{{ t('gptQos.pools') }}</legend>
          <div v-for="(pool, index) in pools" :key="index" class="flex flex-wrap items-end gap-3">
            <label class="min-w-40 flex-1"><span class="input-label">{{ t('gptQos.poolID') }}</span><input v-model="pool.id" class="input" pattern="[a-zA-Z0-9_-]{1,64}" maxlength="64" required /></label>
            <label><span class="input-label">{{ t('gptQos.concurrency') }}</span><input v-model.number="pool.concurrency" class="input" type="number" min="1" max="100000" required /></label>
            <button type="button" class="btn btn-secondary" @click="pools.splice(index, 1)">{{ t('gptQos.remove') }}</button>
          </div>
          <button type="button" class="btn btn-secondary" @click="pools.push({ id: '', concurrency: 1 })">{{ t('gptQos.addPool') }}</button>
        </fieldset>
        <div class="rounded-lg border p-4 dark:border-dark-700"><strong>{{ t('gptQos.effective') }}</strong><p class="mt-2 text-sm">{{ t('gptQos.effectiveHint') }}</p><p class="mt-2 text-xs">Version: {{ config.version }}</p></div>
        <div class="flex flex-wrap items-center gap-3"><button class="btn btn-primary" :disabled="saving">{{ t('gptQos.save') }}</button><button type="button" class="btn btn-secondary" :disabled="saving" @click="rollback">{{ t('gptQos.rollback') }}</button><span v-if="saved" role="status" class="text-green-600">{{ t('gptQos.saved') }}</span></div>
      </form>
      <section class="space-y-3 rounded-lg border p-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold">{{ t('gptQos.metrics') }}</h2><p class="input-hint">{{ t('gptQos.metricsWindow') }}</p>
        <div class="flex flex-wrap items-end gap-3"><label v-for="field in filterFields" :key="field"><span class="input-label">{{ t(`gptQos.${field}`) }}</span><input v-model="filters[field]" class="input" maxlength="128" /></label><button class="btn btn-secondary" type="button" @click="refresh">{{ t('gptQos.refresh') }}</button></div>
        <p v-if="snapshot?.metrics_dropped" role="alert" class="text-amber-700">{{ t('gptQos.metricsDropped') }}: {{ snapshot.metrics_dropped }}</p>
        <p v-if="snapshot && !snapshot.available" role="alert" class="text-amber-700">{{ t('gptQos.redisUnavailable') }}</p>
        <p class="input-hint">{{ t('gptQos.accountTypeHint') }}</p>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm" data-testid="qos-metrics">
            <thead><tr><th class="whitespace-nowrap p-2">{{ t('gptQos.qosGroup') }}</th><th v-for="field in metricFields" :key="field.key" class="whitespace-nowrap p-2">{{ t(`gptQos.${field.label}`) }}</th></tr></thead>
            <template v-for="(row, tier) in snapshot?.tiers" :key="tier">
              <tbody>
                <tr :data-qos-tier="tier" class="border-t dark:border-dark-700">
                  <th scope="row" class="p-2">
                    <button type="button" class="flex items-center gap-2 whitespace-nowrap rounded text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500" :aria-expanded="expandedTiers.has(tier)" :aria-controls="`qos-groups-${tier}`" @click="toggleTier(tier)">
                      <Icon :name="expandedTiers.has(tier) ? 'chevronDown' : 'chevronRight'" size="sm" aria-hidden="true" />
                      <span class="capitalize">{{ tier }}</span>
                      <span class="font-normal text-gray-500">({{ snapshot?.groups?.[tier]?.length || 0 }})</span>
                    </button>
                  </th>
                  <td v-for="field in metricFields" :key="field.key" class="p-2">{{ metricValue(row, field.key) }}</td>
                </tr>
              </tbody>
              <tbody v-if="expandedTiers.has(tier)" :id="`qos-groups-${tier}`" class="bg-gray-50 dark:bg-dark-800">
                <tr v-for="group in snapshot?.groups?.[tier] || []" :key="group.group_id" :data-group-id="group.group_id">
                  <th scope="row" class="whitespace-nowrap py-2 pl-8 pr-2 font-normal">
                    {{ group.group_name || t('gptQos.unnamedGroup') }}
                    <span class="ml-1 text-xs text-gray-500">#{{ group.group_id }}</span>
                  </th>
                  <td v-for="field in metricFields" :key="field.key" class="p-2">{{ metricValue(group, field.key) }}</td>
                </tr>
                <tr v-if="!snapshot?.groups?.[tier]?.length"><td :colspan="metricFields.length + 1" class="p-3 pl-8 text-gray-500">{{ t('gptQos.noGroupMetrics') }}</td></tr>
              </tbody>
            </template>
          </table>
        </div>
        <h3 class="font-medium">{{ t('gptQos.capacity') }}</h3>
        <p v-if="appliedPeak" class="input-hint">{{ t('gptQos.peakStatusHint', { enter: Math.round(appliedPeak.enter_utilization * 100), enterSeconds: appliedPeak.enter_hold_ms / 1000, exit: Math.round(appliedPeak.exit_utilization * 100), exitSeconds: appliedPeak.exit_hold_ms / 1000 }) }}</p>
        <p class="text-sm">{{ t('gptQos.queued') }}: {{ snapshot?.capacity?.queued || 0 }} · {{ t('gptQos.active') }}: {{ snapshot?.capacity?.active || 0 }}</p>
        <p v-if="snapshot?.capacity?.strong_pool?.capacity" class="text-sm">{{ t('gptQos.aggregateStrong') }}: {{ snapshot.capacity.strong_pool.active || 0 }} / {{ snapshot.capacity.strong_pool.capacity }} · {{ t('gptQos.reservedUsed') }}: {{ snapshot.capacity.strong_pool.premium_reserved_used || 0 }} / {{ snapshot.capacity.strong_pool.reserved || 0 }} · {{ t('gptQos.peakStatus') }}: {{ peakStatus(snapshot.capacity.strong_pool, true) }} · {{ peakReason(snapshot.capacity.strong_pool.reason) }}</p>
        <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th v-for="label in poolLabels" :key="label" class="whitespace-nowrap p-2">{{ t(`gptQos.${label}`) }}</th></tr></thead><tbody><tr v-for="pool in snapshot?.capacity?.pools || []" :key="pool.id" :data-pool-id="pool.id"><td class="p-2">{{ pool.display_name || pool.id }}</td><td class="p-2">{{ pool.active }} / {{ pool.capacity }}</td><td class="p-2">{{ pool.strong_active || 0 }} / {{ pool.strong_capacity ?? '—' }}</td><td class="p-2">{{ pool.premium_reserved_used || 0 }} / {{ pool.reserved ?? '—' }}</td><td class="p-2">{{ pool.premium_active || 0 }}</td><td class="p-2">{{ peakStatus(pool) }}</td><td class="p-2">{{ peakReason(pool.reason, pool) }}</td></tr></tbody></table></div>
        <div class="grid gap-3 sm:grid-cols-2"><div><h3 class="font-medium">{{ t('gptQos.downgradeReasons') }}</h3><p v-for="(count, reason) in snapshot?.downgrade_reasons || {}" :key="reason" class="text-sm">{{ reason }}: {{ count }}</p></div><div><h3 class="font-medium">{{ t('gptQos.upstreamShares') }}</h3><p v-for="(count, source) in snapshot?.account_type_requests || {}" :key="source" class="text-sm">{{ accountTypeLabel(source) }}: {{ count }} ({{ sourceShare(count) }}%)</p></div></div>
        <details><summary class="cursor-pointer">{{ t('gptQos.capacity') }}</summary><pre class="mt-3 max-h-80 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ JSON.stringify(snapshot?.capacity || {}, null, 2) }}</pre></details>
      </section>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getQoSConfig, getQoSSnapshot, saveQoSConfig, type QoSConfig, type QoSPoolState, type QoSSnapshot, type QoSTierMetrics } from '@/api/admin/gptQos'
const { t } = useI18n()
const config = ref<QoSConfig | null>(null), snapshot = ref<QoSSnapshot | null>(null)
// The form is a draft. Runtime labels use the observed server policy, with a
// saved-policy fallback for older snapshot responses that omit config.
const savedPeak = ref<QoSConfig['peak'] | null>(null)
const savedVersion = ref(0)
const appliedPeak = computed(() => snapshot.value?.config?.peak ?? savedPeak.value)
const observationFailed = ref(false)
const rolloutGroups = ref('')
const error = ref(''), saving = ref(false), saved = ref(false)
const pools = ref<Array<{ id: string; concurrency: number }>>([])
const filters = reactive({ groupID: '', model: '', effort: '' })
const expandedTiers = reactive(new Set<string>())
function toggleTier(tier: string) { if (expandedTiers.has(tier)) expandedTiers.delete(tier); else expandedTiers.add(tier) }
function metricValue(row: QoSTierMetrics, key: keyof QoSTierMetrics) { return row[key] === undefined ? '—' : Math.round(row[key]) }
const filterFields = ['groupID', 'model', 'effort'] as const
const poolLabels = ['poolID', 'active', 'strongCapacity', 'reservedUsed', 'premiumActive', 'peakStatus', 'peakReason'] as const
function peakStatus(pool: Partial<QoSPoolState>, aggregate = false): string {
  if (observationFailed.value || !snapshot.value?.available || !appliedPeak.value) return t('gptQos.peakUnknown')
  if (snapshot.value.config && savedPeak.value && snapshot.value.config.version <= savedVersion.value && snapshot.value.config.peak.enabled !== savedPeak.value.enabled) return t('gptQos.peakConfigMismatch')
  if (!appliedPeak.value.enabled) return t('gptQos.peakOff')
  const capacity = aggregate ? pool.capacity : pool.strong_capacity
  if (capacity === 0) return t('gptQos.peakNotApplicable')
  if (capacity === undefined) return t('gptQos.peakNoData')
  if (pool.peak || (!aggregate && snapshot.value.capacity?.strong_pool?.peak)) return t('gptQos.peakActive')
  return t('gptQos.peakWaiting')
}
function peakReason(reason?: string, pool?: Partial<QoSPoolState>): string {
  if (observationFailed.value || !snapshot.value?.available) return t('gptQos.peakUnknown')
  if (appliedPeak.value && !appliedPeak.value.enabled) return t('gptQos.peakOff')
  if (pool && !pool.peak && (pool.strong_capacity || 0) > 0 && snapshot.value.capacity?.strong_pool?.peak) return t('gptQos.peakAggregateTriggered')
  if (reason === 'below_threshold_or_hysteresis_hold') return t('gptQos.peakBelowThreshold')
  if (reason === 'aggregate_strong_utilization_above_threshold_stable') return t('gptQos.peakAggregateTriggered')
  if (reason === 'strong_utilization_above_threshold_stable') return t('gptQos.peakPoolTriggered')
  return reason || t('gptQos.peakNoData')
}
const sourceShare = (count: number) => { const total = Object.values(snapshot.value?.account_type_requests || {}).reduce((sum, n) => sum + n, 0); return total ? Math.round(count * 1000 / total) / 10 : 0 }
const accountTypeLabel = (source: string) => t(source === 'apikey' ? 'gptQos.apiRequests' : source === 'oauth' ? 'gptQos.oauthRequests' : 'gptQos.unclassifiedRequests')
const flags = [{ key: 'quality_routing_enabled', label: 'qualityRouting' }, { key: 'enabled', label: 'qosEnabled' }, { key: 'capacity_enabled', label: 'capacityEnabled' }] as const
const numberFields = [
  { key: 'premium_weight', label: 'premiumWeight', min: 1, max: 100 }, { key: 'standard_weight', label: 'standardWeight', min: 1, max: 100 }, { key: 'economy_weight', label: 'economyWeight', min: 1, max: 100 },
  { key: 'max_waiting', label: 'maxWaiting', min: 1, max: 2000 }, { key: 'request_budget_ms', label: 'requestBudget', min: 1000, max: 3600000 }, { key: 'high_effort_budget_ms', label: 'highBudget', min: 1000, max: 7200000 },
  { key: 'max_attempts', label: 'maxAttempts', min: 1, max: 20 }, { key: 'log_sample_rate', label: 'sampleRate', min: 0, max: 1, step: 0.01 }
] as const
const peakFields = [
  { key: 'enter_utilization', label: 'enter', min: 0.01, max: 1, step: 0.01 }, { key: 'exit_utilization', label: 'exit', min: 0, max: 0.99, step: 0.01 }, { key: 'protection_ratio', label: 'ratio', min: 0, max: 0.9, step: 0.01 },
  { key: 'enter_hold_ms', label: 'enterHold', min: 100, max: 600000, step: 100 }, { key: 'exit_hold_ms', label: 'exitHold', min: 100, max: 600000, step: 100 }
] as const
const metricFields = [
  { key: 'Requests', label: 'requests' }, { key: 'Attempts', label: 'attempts' }, { key: 'Successes', label: 'successes' }, { key: 'FirstSuccesses', label: 'firstSuccess' }, { key: 'first_success_rate', label: 'firstSuccessRate' }, { key: 'Canceled', label: 'canceled' }, { key: 'StrongRequests', label: 'strongHits' }, { key: 'strong_hit_rate', label: 'strongHitRate' }, { key: 'Downgrades', label: 'downgrades' }, { key: 'APIRequests', label: 'apiRequests' }, { key: 'OAuthRequests', label: 'oauthRequests' },
  { key: 'average_queue_ms', label: 'average' }, { key: 'p50_queue_ms', label: 'p50' }, { key: 'p95_queue_ms', label: 'p95' }, { key: 'p99_queue_ms', label: 'p99' }
] as const
let observationGeneration = 0
async function refresh() {
  const generation = ++observationGeneration
  try {
    const next = await getQoSSnapshot({ group_id: filters.groupID, model: filters.model, effort: filters.effort })
    if (generation !== observationGeneration) return
    snapshot.value = next
    observationFailed.value = false
  } catch {
    if (generation !== observationGeneration) return
    observationFailed.value = true
    error.value = t('gptQos.failed')
  }
}
async function save() {
  if (!config.value) return
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    if (new Set(pools.value.map(p => p.id)).size !== pools.value.length) throw new Error('Duplicate pool ID')
    const latest = await getQoSConfig()
    config.value = await saveQoSConfig({
      ...config.value,
      // This page edits global policy only. Preserve group QoS saved from a
      // group dialog after this page was opened.
      groups: latest.groups,
      group_ids: rolloutGroups.value.split(',').map(s => s.trim()).filter(Boolean).map(Number),
      resource_pools: Object.fromEntries(pools.value.map(p => [p.id, { concurrency: p.concurrency }]))
    })
    savedPeak.value = { ...config.value.peak }
    savedVersion.value = config.value.version
    saved.value = true
    await refresh()
  } catch (e: unknown) {
    error.value = e && typeof e === 'object' && 'message' in e && typeof e.message === 'string' ? e.message : t('gptQos.failed')
  } finally {
    saving.value = false
  }
}
async function rollback() { if (!config.value) return; config.value.enabled = false; config.value.quality_routing_enabled = false; config.value.capacity_enabled = false; config.value.peak.enabled = false; await save() }
onMounted(async () => { try { config.value = await getQoSConfig(); savedPeak.value = { ...config.value.peak }; savedVersion.value = config.value.version; rolloutGroups.value = (config.value.group_ids || []).join(','); pools.value = Object.entries(config.value.resource_pools).map(([id, p]) => ({ id, ...p })); await refresh() } catch { error.value = t('gptQos.failed') } })
</script>
