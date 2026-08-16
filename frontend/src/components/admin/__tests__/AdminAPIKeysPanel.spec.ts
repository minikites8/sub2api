import { defineComponent } from 'vue'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AdminAPIKeysPanel from '../AdminAPIKeysPanel.vue'

const { listAdminApiKeys, createAdminApiKey, updateAdminApiKey, deleteAdminApiKeyById, getAllProxies } = vi.hoisted(() => ({
  listAdminApiKeys: vi.fn(),
  createAdminApiKey: vi.fn(),
  updateAdminApiKey: vi.fn(),
  deleteAdminApiKeyById: vi.fn(),
  getAllProxies: vi.fn()
}))

vi.mock('@/api/admin/settings', () => ({
  listAdminApiKeys,
  createAdminApiKey,
  updateAdminApiKey,
  deleteAdminApiKeyById
}))

vi.mock('@/api/admin/proxies', () => ({ getAll: getAllProxies }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const ConfirmDialogStub = defineComponent({
  props: { show: Boolean },
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-testid="confirm-dialog"><button data-testid="confirm" @click="$emit(\'confirm\')">confirm</button></div>'
})

function key(overrides: Record<string, unknown> = {}) {
  return {
    id: 'key_1',
    name: 'Pool worker',
    permission: 'auto_pool',
    masked_key: 'admin-abcd1234...efgh',
    created_at: '2026-08-17T00:00:00Z',
    account_defaults: {
      proxy_mode: 'none',
      codex_fingerprint_mode: 'off',
      revoke_other_sessions: false
    },
    ...overrides
  }
}

function mountPanel() {
  return mount(AdminAPIKeysPanel, {
    global: {
      stubs: {
        ConfirmDialog: ConfirmDialogStub,
        Icon: true
      }
    }
  })
}

async function chooseOption(wrapper: ReturnType<typeof mountPanel>, id: string, label: string) {
  await wrapper.find(`#${id}`).trigger('click')
  const option = Array.from(document.body.querySelectorAll<HTMLElement>('.select-option'))
    .find(item => item.textContent?.includes(label))
  if (!option) throw new Error(`Option not found: ${label}`)
  await new DOMWrapper(option).trigger('click')
}

describe('AdminAPIKeysPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listAdminApiKeys.mockResolvedValue([key()])
    createAdminApiKey.mockResolvedValue({
      key: 'admin-new-secret',
      api_key: key({ id: 'key_2', name: 'Full access', permission: 'full', masked_key: 'admin-new...cret' })
    })
    updateAdminApiKey.mockResolvedValue(key())
    getAllProxies.mockResolvedValue([])
    deleteAdminApiKeyById.mockResolvedValue({ message: 'deleted' })
  })

  it('loads keys and creates a key with one-time secret display', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(listAdminApiKeys).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Pool worker')

    await wrapper.find('#admin-api-key-name').setValue('Full access')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createAdminApiKey).toHaveBeenCalledWith({
      name: 'Full access',
      permission: 'auto_pool',
      account_defaults: {
        proxy_mode: 'none',
        codex_fingerprint_mode: 'off',
        revoke_other_sessions: false
      }
    })
    expect(wrapper.text()).toContain('admin-new-secret')
    expect(wrapper.text()).toContain('Full access')
  })

  it('deletes the selected key after confirmation', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="delete-key"]').trigger('click')
    await wrapper.find('[data-testid="confirm"]').trigger('click')
    await flushPromises()

    expect(deleteAdminApiKeyById).toHaveBeenCalledWith('key_1')
    expect(wrapper.find('tbody').exists()).toBe(false)
  })

  it('edits account import defaults', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="edit-key"]').trigger('click')
    await chooseOption(wrapper, 'admin-api-key-proxy-mode', 'admin.settings.adminApiKey.proxyRandom')
    await chooseOption(wrapper, 'admin-api-key-fingerprint', 'admin.settings.adminApiKey.fingerprintSession')
    await wrapper.find('#admin-api-key-revoke-sessions').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateAdminApiKey).toHaveBeenCalledWith('key_1', {
      name: 'Pool worker',
      permission: 'auto_pool',
      account_defaults: {
        proxy_mode: 'random',
        codex_fingerprint_mode: 'session',
        revoke_other_sessions: true
      }
    })
  })
})
