import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAISessionsModal from '../OpenAISessionsModal.vue'

const { listOpenAISessions, revokeOpenAISession } = vi.hoisted(() => ({
  listOpenAISessions: vi.fn(),
  revokeOpenAISession: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  listOpenAISessions,
  revokeOpenAISession
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /></div>'
})

const ConfirmDialogStub = defineComponent({
  props: {
    show: Boolean,
    title: String,
    confirmText: String,
    cancelText: String
  },
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-testid="confirm-dialog" :data-title="title">
      <button data-testid="confirm" type="button" @click="$emit('confirm')">{{ confirmText }}</button>
      <button data-testid="cancel" type="button" @click="$emit('cancel')">{{ cancelText }}</button>
    </div>
  `
})

function device(overrides: Record<string, unknown>) {
  return {
    display_name: 'Device',
    is_trusted_device: false,
    is_current_device: false,
    can_untrust: false,
    last_signed_in_timestamp_second: 1,
    ...overrides
  }
}

function mountModal() {
  return mount(OpenAISessionsModal, {
    props: {
      show: true,
      account: { id: 42, name: 'OpenAI OAuth' } as any
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Icon: true
      }
    }
  })
}

describe('OpenAISessionsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listOpenAISessions.mockResolvedValue({
      show_session_manager: true,
      fetched_at: 1,
      devices: [
        device({ display_name: 'Current', session_id: 'current', is_current_device: true }),
        device({ display_name: 'Laptop', session_id: 'laptop' }),
        device({ display_name: 'Phone', session_id: 'phone' }),
        device({ display_name: 'Unknown session', session_id: '' })
      ]
    })
    revokeOpenAISession.mockResolvedValue({ revoked: true })
  })

  it('一键踢出其他设备时保留当前设备并逐台撤销', async () => {
    const wrapper = mountModal()
    await flushPromises()

    const bulkButton = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.sessions.revokeOthers'))
    expect(bulkButton).toBeTruthy()
    await bulkButton!.trigger('click')

    const confirmDialog = wrapper.find('[data-title="admin.accounts.sessions.revokeOthersTitle"]')
    expect(confirmDialog.exists()).toBe(true)
    await confirmDialog.find('[data-testid="confirm"]').trigger('click')
    await flushPromises()

    expect(revokeOpenAISession).toHaveBeenCalledTimes(2)
    expect(revokeOpenAISession.mock.calls).toEqual([
      [42, 'laptop'],
      [42, 'phone']
    ])
    expect(listOpenAISessions).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('批量撤销遇到失败时继续处理后续设备并提示失败数量', async () => {
    revokeOpenAISession
      .mockRejectedValueOnce(new Error('upstream failed'))
      .mockResolvedValueOnce({ revoked: true })

    const wrapper = mountModal()
    await flushPromises()
    const bulkButton = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.sessions.revokeOthers'))
    await bulkButton!.trigger('click')
    await wrapper.find('[data-title="admin.accounts.sessions.revokeOthersTitle"] [data-testid="confirm"]').trigger('click')
    await flushPromises()

    expect(revokeOpenAISession).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.accounts.sessions.revokeOthersFailed:{"count":1}')
    wrapper.unmount()
  })
})
