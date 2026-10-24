<template>
  <section class="card space-y-5 p-4 sm:p-6" data-testid="gpt-auto-pricing">
    <h2 class="text-xl font-semibold text-gray-900 dark:text-white">gpt-auto · {{ t('gptAuto.pricing.title') }}</h2>
    <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('gptAuto.pricing.description') }}</p>
    <p v-if="error" role="alert" class="text-red-600 dark:text-red-400">{{ error }}</p>
    <template v-if="state">
      <p class="text-sm">{{ t(state.ready ? 'gptAuto.pricing.ready' : 'gptAuto.pricing.missing') }} · v{{ state.pricing.version }} · {{ state.pricing.updated_at || '—' }}</p>
      <p class="text-sm font-medium">{{ t('gptAuto.pricing.active') }}: {{ t(`gptAuto.pricing.modes.${state.pricing.mode}`) }}</p>
      <form class="space-y-5" @submit.prevent="preview">
        <label class="block"><span class="input-label">{{ t('gptAuto.pricing.mode') }}</span>
          <Select v-model="draft.mode" :options="modes" data-testid="pricing-mode" />
        </label>
        <div v-if="draft.mode === 'virtual_model_fixed_rates'" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <label v-for="field in rates" :key="field"><span class="input-label">{{ t(`gptAuto.pricing.${field}`) }} · USD / 1M</span>
            <input v-model="draft[field]" :data-testid="field" class="input" type="number" min="0" max="1000" step="0.000001" :required="field !== 'cache_write_price_per_mtok'" />
          </label>
        </div>
        <p class="text-xs text-gray-500">{{ t('gptAuto.pricing.cacheWriteHint') }}</p>
        <h3 class="font-semibold">{{ t('gptAuto.pricing.simulator') }}</h3>
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <label><span class="input-label">{{ t('gptAuto.pricing.group') }}</span><Select v-model="groupID" :options="groupOptions" data-testid="pricing-group" /></label>
          <label><span class="input-label">{{ t('gptAuto.pricing.executed') }}</span><Select v-model="executedModel" :options="modelOptions" /></label>
          <label v-for="field in tokenFields" :key="field"><span class="input-label">{{ t(`gptAuto.pricing.${field}`) }}</span><input v-model.number="tokens[field]" class="input" type="number" min="0" max="10000000" step="1" required /></label>
        </div>
        <p v-if="group" class="text-sm" data-testid="effective-group-price">{{ t(`gptAuto.pricing.sources.${group.pricing_source || 'global'}`) }} · {{ t('gptAuto.pricing.savedGroupRates') }}: {{ group.effective_input_price_per_mtok ?? '—' }} / {{ group.effective_cached_input_price_per_mtok ?? '—' }} / {{ group.effective_output_price_per_mtok ?? '—' }} USD / 1M</p>
        <p v-if="group" class="text-xs text-amber-700 dark:text-amber-300">{{ t(group.gpt_auto_enabled ? 'gptAuto.group.enabled' : 'gptAuto.group.disabled') }} {{ !state.live_enabled ? t('gptAuto.group.globalOff') : '' }} {{ group.requires_key_allowlist ? t('gptAuto.group.internalOnly') : '' }} {{ group.excluded_by_rollout || group.model_allowlist_blocks_gpt_auto ? t('gptAuto.group.restricted') : '' }}</p>
        <p class="text-xs text-gray-500">{{ t('gptAuto.pricing.estimateHint') }}</p>
        <button class="btn btn-secondary" :disabled="busy" data-testid="preview-pricing">{{ t('gptAuto.pricing.preview') }}</button>
      </form>
      <template v-if="simulation">
        <p class="text-sm">{{ t(`gptAuto.pricing.sources.${simulation.pricing_source}`) }} · ×{{ simulation.rate_multiplier }}</p>
        <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ t('gptAuto.pricing.executed') }}</th><th v-for="field in moneyFields" :key="field" class="p-2">{{ t(`gptAuto.pricing.${field}`) }}</th><th class="p-2">{{ t('gptAuto.pricing.margin') }}</th></tr></thead>
          <tbody><tr v-for="row in simulation.rows" :key="row.model"><td class="p-2 font-mono">{{ row.model }}</td><td v-for="field in moneyFields" :key="field" class="p-2 tabular-nums">{{ money(row[field]) }}</td><td class="p-2">{{ row.gross_margin == null ? '—' : `${(row.gross_margin * 100).toFixed(2)}%` }}</td></tr></tbody></table></div>
        <p v-if="simulation.warnings.length" role="alert" class="rounded-lg bg-amber-50 p-3 text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">{{ t('gptAuto.pricing.risk') }}<br />{{ simulation.warnings.map(warningLabel).join(' · ') }}</p>
        <label class="flex items-start gap-2"><input v-model="confirmed" type="checkbox" data-testid="confirm-pricing" /><span class="text-sm">{{ t('gptAuto.pricing.confirm') }}</span></label>
        <button class="btn btn-primary" :disabled="busy || !confirmed" data-testid="save-pricing" @click="save">{{ t('gptAuto.pricing.save') }}</button>
      </template>
      <p v-if="saved" role="status" class="text-green-700 dark:text-green-400">{{ t('gptAuto.pricing.saved') }}</p>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { getPricing, savePricing, simulatePricing, type GPTAutoPricing, type GPTAutoPricingState, type GPTAutoPriceSimulation } from '@/api/admin/gptAuto'
const { t } = useI18n()
const state = ref<GPTAutoPricingState | null>(null)
const draft = reactive<GPTAutoPricing>({ mode: 'virtual_model_fixed_rates', input_price_per_mtok: null, cached_input_price_per_mtok: null, output_price_per_mtok: null, cache_write_price_per_mtok: null, version: 0 })
const rates = ['input_price_per_mtok', 'cached_input_price_per_mtok', 'output_price_per_mtok', 'cache_write_price_per_mtok'] as const
const tokenFields = ['input_tokens', 'cached_input_tokens', 'output_tokens'] as const
const moneyFields = ['customer_charge_usd', 'estimated_upstream_cost_usd', 'estimated_classifier_cost_usd', 'gross_profit_usd', 'fully_loaded_profit_usd'] as const
const tokens = reactive({ input_tokens: 1000, cached_input_tokens: 200, output_tokens: 500 })
const groupID = ref(0), executedModel = ref(''), busy = ref(false), confirmed = ref(false), saved = ref(false), error = ref('')
const simulation = ref<GPTAutoPriceSimulation | null>(null)
const modes = computed(() => ['virtual_model_fixed_rates', 'legacy_executed_model_multiplier'].map(value => ({ value, label: t(`gptAuto.pricing.modes.${value}`) })))
const groupOptions = computed(() => [{ value: 0, label: t('gptAuto.pricing.sources.global') }, ...(state.value?.groups ?? []).map(g => ({ value: g.group_id, label: g.name }))])
const modelOptions = computed(() => [{ value: '', label: t('gptAuto.pricing.allModels') }, ...['gpt-5.6-luna', 'gpt-6-luna', 'gpt-6.1-sol'].map(value => ({ value, label: value }))])
const group = computed(() => state.value?.groups.find(g => g.group_id === groupID.value))
const money = (value?: string) => value == null ? '—' : `$${Number(value).toFixed(10).replace(/0+$/, '').replace(/\.$/, '.00')}`
const warningLabel = (warning: string) => { const [kind, model] = warning.split(':'); return `${t(`gptAuto.pricing.warnings.${kind}`)}${model ? ` (${model})` : ''}` }
const failure = (e: unknown) => { error.value = (e as { message?: string }).message || t('gptAuto.pricing.failed') }
function candidate(): GPTAutoPricing { return { ...draft, ...Object.fromEntries(rates.map(k => [k, draft[k] === '' || draft[k] == null ? null : String(draft[k])])) } }
watch([draft, tokens, groupID, executedModel], () => { simulation.value = null; confirmed.value = false; saved.value = false }, { deep: true })
async function preview() {
  busy.value = true; error.value = ''; confirmed.value = false
  try { simulation.value = await simulatePricing({ pricing: candidate(), group_id: groupID.value || undefined, ...tokens, executed_model: executedModel.value }) } catch (e) { failure(e) } finally { busy.value = false }
}
async function save() {
  if (!confirmed.value || !simulation.value || !state.value) return
  busy.value = true; error.value = ''
  try { state.value = await savePricing(candidate(), state.value.pricing.version); Object.assign(draft, state.value.pricing); simulation.value = null; confirmed.value = false; await nextTick(); saved.value = true } catch (e) { failure(e) } finally { busy.value = false }
}
onMounted(async () => { try { state.value = await getPricing(); Object.assign(draft, state.value.pricing) } catch (e) { failure(e) } })
</script>
