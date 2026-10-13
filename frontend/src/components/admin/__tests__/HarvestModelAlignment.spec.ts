import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import Panel from '@/components/admin/HarvestPolicyPanel.vue'
import Console from '@/components/admin/HarvestManualConsole.vue'

const { getSettings, updateSettings, stream } = vi.hoisted(() => ({ getSettings: vi.fn(), updateSettings: vi.fn(), stream: vi.fn() }))
vi.mock('@/api/admin/settings', () => ({ getSettings, updateSettings }))
vi.mock('@/api/admin/accounts', () => ({ streamManualCodexHarvest: stream }))
vi.mock('@/components/admin/HarvestPoolSchedulingPanel.vue', () => ({ default: { template: '<section />' } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh-CN' } }) }))
enableAutoUnmount(afterEach)

const live = ['gpt-6-astra', 'gpt-6-sol', 'gpt-6.1-sol']
const settings = { openai_codex_ticket_enabled: true, openai_codex_ticket_models: live, openai_codex_ticket_harvest_scope: { mode: 'all', group_ids: [] } }
beforeEach(() => { vi.resetAllMocks(); getSettings.mockResolvedValue(settings); updateSettings.mockResolvedValue(settings); stream.mockResolvedValue(undefined) })
const globals = { stubs: { RouterLink: { template: '<a><slot /></a>' }, Icon: true, Toggle: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input type="checkbox" :checked="modelValue" />' } } }
function panel(selected = live, available = live) { getSettings.mockResolvedValue({ ...settings, openai_codex_ticket_models: selected }); return mount(Panel, { props: { models: available }, global: globals }) }

describe('effective harvest model alignment', () => {
  it('shows each effective model and preserves exact IDs on save', async () => {
    const w = panel(); await flushPromises()
    for (const model of live) expect(w.get<HTMLInputElement>(`[data-model="${model}"]`).element.checked).toBe(true)
    expect(w.find('[data-model="gpt-5.6-sol"]').exists()).toBe(false)
    await w.get('[data-model="gpt-6.1-sol"]').setValue(false)
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual(['gpt-6-astra', 'gpt-6-sol'])
    expect(w.find('[data-model="gpt-6.1-sol"]').exists()).toBe(true)
  })
  it('renders unselected runtime options while preserving an explicit empty selection', async () => {
    const w = panel([], live); await flushPromises()
    for (const model of live) expect(w.get<HTMLInputElement>(`[data-model="${model}"]`).element.checked).toBe(false)
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual([])
  })
  it('uses runtime models when the legacy settings response omits its model field', async () => {
    getSettings.mockResolvedValue({ openai_codex_ticket_enabled: true })
    const w = mount(Panel, { props: { models: live }, global: globals }); await flushPromises()
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual(live)
  })
  it('adds a complete custom model ID and deduplicates it', async () => {
    const w = panel(); await flushPromises()
    await w.get('[data-testid="harvest-model-input"]').setValue('  gpt-6.2-sol  ')
    await w.get('[data-testid="add-harvest-model"]').trigger('click')
    await w.get('[data-testid="harvest-model-input"]').setValue('gpt-6.2-sol')
    await w.get('[data-testid="add-harvest-model"]').trigger('click')
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual([...live, 'gpt-6.2-sol'])
  })
  it('accepts asynchronously arriving runtime models before editing', async () => {
    getSettings.mockResolvedValue({ openai_codex_ticket_enabled: true })
    const w = mount(Panel, { props: { models: [] }, global: globals }); await flushPromises()
    await w.setProps({ models: live })
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual(live)
  })
  it('waits for the authoritative list when both responses are still missing models', async () => {
    getSettings.mockResolvedValue({ openai_codex_ticket_enabled: true })
    const w = mount(Panel, { global: globals }); await flushPromises()
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings).not.toHaveBeenCalled()
    expect(w.get('[data-testid="save-harvest-policy"]').attributes()).toHaveProperty('disabled')
    await w.setProps({ models: live })
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual(live)
  })
  it('preserves a draft model selection while runtime polling adds an option', async () => {
    getSettings.mockResolvedValue({ openai_codex_ticket_enabled: true })
    const w = mount(Panel, { props: { models: live }, global: globals }); await flushPromises()
    await w.get('[data-model="gpt-6-sol"]').setValue(false)
    await w.setProps({ models: [...live, 'gpt-6.2-sol'] })
    await w.get('form').trigger('submit.prevent'); await flushPromises()
    expect(updateSettings.mock.calls[0][0].openai_codex_ticket_models).toEqual(['gpt-6-astra', 'gpt-6.1-sol'])
    expect(w.get<HTMLInputElement>('[data-model="gpt-6.2-sol"]').element.checked).toBe(false)
  })
})

async function consoleView(models: string[]) {
  const w = mount(Console, { props: { accounts: [{ id: 41, name: 'fixture', status: 'active', schedulable: true, tickets: [], ready_count: 0, blocked_count: 0 }], models }, global: globals })
  await w.get('[data-testid="manual-account-input"]').trigger('focus')
  await w.get('[data-testid="manual-account-41"]').trigger('click')
  return w
}
describe('manual harvest runtime defaults', () => {
  it('refreshes the default selection when the effective model set changes', async () => {
    const w = await consoleView(live)
    await w.setProps({ models: ['gpt-6-astra'] })
    await w.get('[data-testid="manual-start"]').trigger('click'); await flushPromises()
    expect(stream.mock.calls[0][1].models).toEqual(['gpt-6-astra'])
  })
  it('clears removed models when the effective list becomes empty', async () => {
    const w = await consoleView(live)
    await w.setProps({ models: [] })
    await w.get('[data-testid="manual-start"]').trigger('click'); await flushPromises()
    expect(stream).not.toHaveBeenCalled()
    expect(w.text()).not.toContain('gpt-6-sol')
  })
})
