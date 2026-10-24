import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Pricing from '@/components/admin/gptAuto/GPTAutoPricingPanel.vue'
import GroupControl from '@/components/admin/gptAuto/GPTAutoGroupControl.vue'
import { createModelAllowlistState, buildModelAllowlistConfig } from '../groupModelAllowlist'
const api = vi.hoisted(() => ({ getPricing: vi.fn(), savePricing: vi.fn(), simulatePricing: vi.fn() }))
vi.mock('@/api/admin/gptAuto', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))
const state = () => ({ ready: true, live_enabled: false, internal_only: true, groups: [], pricing: { mode: 'virtual_model_fixed_rates', input_price_per_mtok: '2', cached_input_price_per_mtok: '0.5', output_price_per_mtok: '8', cache_write_price_per_mtok: null, version: 2 } })
const global = { stubs: { RouterLink: { template: '<a><slot /></a>' }, Select: { props: ['modelValue', 'options'], template: '<select><option v-for="o in options">{{ o.label }}</option></select>' } } }
beforeEach(() => { vi.clearAllMocks(); api.getPricing.mockResolvedValue(state()); api.savePricing.mockResolvedValue({ ...state(), pricing: { ...state().pricing, version: 3 } }); api.simulatePricing.mockResolvedValue({ pricing_source: 'global', rate_multiplier: 1, warnings: ['gross_loss:gpt-6.1-sol'], rows: [{ model: 'gpt-6.1-sol', customer_charge_usd: '0.0057', estimated_upstream_cost_usd: '0.01', estimated_classifier_cost_usd: '0.0001', gross_profit_usd: '-0.0043', fully_loaded_profit_usd: '-0.0044', gross_margin: -0.754386 }] }) })
describe('GPT Auto independent pricing', () => {
 it('requires preview and confirmation, sends decimal strings and version, shows negative margin', async () => {
  const w = mount(Pricing, { global }); await flushPromises()
  expect(w.text()).toContain('gptAuto.pricing.modes.virtual_model_fixed_rates')
  expect(w.find('[data-testid="save-pricing"]').exists()).toBe(false)
  await w.get('form').trigger('submit'); await flushPromises()
  expect(w.text()).toContain('$0.0057'); expect(w.text()).toContain('$-0.0043')
  expect(w.get('[data-testid="save-pricing"]').attributes('disabled')).toBeDefined()
  await w.get('[data-testid="confirm-pricing"]').setValue(true)
  await w.get('[data-testid="save-pricing"]').trigger('click'); await flushPromises()
  expect(api.savePricing).toHaveBeenCalledWith(expect.objectContaining({ input_price_per_mtok: '2', cached_input_price_per_mtok: '0.5', output_price_per_mtok: '8' }), 2)
  expect(w.text()).toContain('v3')
 })
 it('invalidates approval when tariff changes and displays server validation failures', async () => {
  const w = mount(Pricing, { global }); await flushPromises()
  await w.get('form').trigger('submit'); await flushPromises()
  await w.get('[data-testid="confirm-pricing"]').setValue(true)
  await w.get('[data-testid="input_price_per_mtok"]').setValue('3')
  expect(w.find('[data-testid="save-pricing"]').exists()).toBe(false)
  api.simulatePricing.mockRejectedValueOnce(new Error('invalid tariff'))
  await w.get('form').trigger('submit'); await flushPromises()
  expect(w.get('[role="alert"]').text()).toBe('invalid tariff')
 })
 it('group defaults off, explains global and internal restrictions and adds the virtual permission', async () => {
  const allowlist = createModelAllowlistState({ enabled: true, models: ['gpt-6-luna'] })
  const w = mount(GroupControl, { global, props: { modelValue: false, allowlist } }); await flushPromises()
  expect(w.text()).toContain('gptAuto.group.disabled'); expect(w.text()).toContain('gptAuto.group.globalOff'); expect(w.text()).toContain('gptAuto.group.internalOnly')
  await w.get('[data-testid="group-gpt-auto-toggle"]').trigger('click')
  expect(w.emitted('update:modelValue')?.[0]).toEqual([true])
  Object.assign(allowlist, w.emitted('update:allowlist')?.[0]?.[0])
  expect(buildModelAllowlistConfig(allowlist).models).toContain('gpt-auto')
  expect(buildModelAllowlistConfig(allowlist).models).toContain('gpt-6-luna')
 })
})
