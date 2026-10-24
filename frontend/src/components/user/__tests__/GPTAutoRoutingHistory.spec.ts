import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GPTAutoRoutingHistory from '../GPTAutoRoutingHistory.vue'

const api = vi.hoisted(() => ({
  getStatus: vi.fn(),
  listRoutingLogs: vi.fn(),
  countRoutingLogExport: vi.fn(),
  exportRoutingLogs: vi.fn(),
}))

vi.mock('@/api/gptAuto', () => ({ gptAutoUserAPI: api, default: api }))
vi.mock('@/utils/apiError', () => ({ extractApiErrorMessage: (_err: unknown, fallback: string) => fallback }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const SelectStub = {

    props: ['modelValue', 'options'],
    emits: ['update:modelValue', 'change'],
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\', $event.target.value)"><option v-for="o in options" :key="String(o.value)" :value="o.value">{{ o.label }}</option></select>',
  }

const mountHistory = () => mount(GPTAutoRoutingHistory, { global: { stubs: { Pagination: true, Select: SelectStub } } })

beforeEach(() => {
  vi.clearAllMocks()
  api.getStatus.mockResolvedValue({ has_history: true, virtual_model: 'gpt-auto', export_enabled: true, export_schema_version: 'gpt-auto-export-v1' })
  api.listRoutingLogs.mockResolvedValue({
    items: [{ created_at: '2026-10-20T10:00:00Z', request_id: 'req-1', endpoint: 'responses', requested_model: 'gpt-auto', executed_model: 'gpt-6-luna',
      reasoning_effort: 'medium', status: 'succeeded', total_latency_ms: 812, fallback: true, routing_explanation: 'Automatic selection was unavailable; a compatible default model was used.' }],
    total: 1, page: 1, page_size: 20,
  })
})

describe('GPTAutoRoutingHistory', () => {
  it('stays hidden for customers without gpt-auto history', async () => {
    api.getStatus.mockResolvedValue({ has_history: false, virtual_model: 'gpt-auto', export_enabled: true, export_schema_version: 'v1' })
    const wrapper = mountHistory()
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-history"]').exists()).toBe(false)
    expect(api.listRoutingLogs).not.toHaveBeenCalled()
  })

  it('lists own requests and never sends an owner id', async () => {
    const wrapper = mountHistory()
    await flushPromises()
    expect(wrapper.text()).toContain('gpt-6-luna')
    expect(wrapper.text()).toContain('gptAutoHistory.fallback')
    const params = api.listRoutingLogs.mock.calls[0][0]
    expect(Object.keys(params).sort()).toEqual(['end_time', 'page', 'page_size', 'start_time'])
  })

  it('exports through the server after checking the row count', async () => {
    api.countRoutingLogExport.mockResolvedValue({ rows: 3, truncated: false, max_rows: 100000 })
    api.exportRoutingLogs.mockResolvedValue({ blob: new Blob(['a,b']), filename: 'gpt-auto-routing-customer-x.csv' })
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:x'), revokeObjectURL: vi.fn() })
    const wrapper = mountHistory()
    await flushPromises()
    await wrapper.find('[data-testid="gpt-auto-history-format"]').setValue('jsonl')
    await wrapper.find('[data-testid="gpt-auto-history-export"]').trigger('click')
    await flushPromises()
    expect(api.exportRoutingLogs).toHaveBeenCalledWith(expect.objectContaining({ format: 'jsonl' }))
    expect(Object.keys(api.exportRoutingLogs.mock.calls[0][0]).sort()).toEqual(['end_time', 'format', 'start_time'])
    expect(wrapper.find('[data-testid="gpt-auto-history-notice"]').text()).toBe('gptAutoHistory.exported')
  })

  it('hides export controls when the administrator disabled customer export', async () => {
    api.getStatus.mockResolvedValue({ has_history: true, virtual_model: 'gpt-auto', export_enabled: false, export_schema_version: 'v1' })
    const wrapper = mountHistory()
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-auto-history-export"]').exists()).toBe(false)
  })
})
