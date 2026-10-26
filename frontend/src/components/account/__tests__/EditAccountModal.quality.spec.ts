import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const { updateAccountMock } = vi.hoisted(() => ({ updateAccountMock: vi.fn() }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: false }) }))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getManagementCapabilities: vi.fn().mockResolvedValue({ web_search_enabled: false, account_quota_notify_enabled: false }),
      update: updateAccountMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const groups = [
  { id: 3, name: 'Group A', platform: 'anthropic', status: 'active' },
  { id: 5, name: 'Group B', platform: 'anthropic', status: 'active' }
]

const account = () => ({
  id: 12, name: 'Claude key', notes: '', platform: 'anthropic', type: 'apikey',
  credentials: { api_key: 'sk-test', base_url: 'https://api.anthropic.com' }, extra: {},
  proxy_id: null, concurrency: 1, priority: 1, rate_multiplier: 1, status: 'active',
  group_ids: [3, 5], expires_at: null, auto_pause_on_expired: false,
  account_groups: [
    { account_id: 12, group_id: 3, priority: 1, created_at: '' },
    { account_id: 12, group_id: 5, priority: 2, allowed_models: ['claude-sonnet-4-6'], created_at: '' }
  ]
})

function mountModal(overrides: Record<string, unknown> = {}) {
  return mount(EditAccountModal, {
    props: { show: true, account: { ...account(), ...overrides }, proxies: [], groups },
    global: { stubs: {
      BaseDialog: BaseDialogStub, Select: true, Icon: true, ProxySelector: true,
      GroupSelector: true, ModelWhitelistSelector: true
    } }
  })
}

const submit = async (wrapper: ReturnType<typeof mountModal>) => {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalled())
  return updateAccountMock.mock.calls[0][1] as Record<string, unknown>
}

describe('EditAccountModal quality metadata', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    updateAccountMock.mockResolvedValue(account())
  })

  it('loads Strong from the typed account field when extra is partial', () => {
    const wrapper = mountModal({
      platform: 'openai', type: 'apikey', quality_tier: 'strong',
      extra: { openai_passthrough: true }
    })
    expect(wrapper.findComponent({ name: 'AccountQualityFields' }).props('tier')).toBe('strong')
  })

  it('does not send Unknown when changing another setting on a Strong account', async () => {
    const wrapper = mountModal({
      platform: 'openai', type: 'apikey', quality_tier: 'strong',
      extra: { openai_passthrough: true }
    })
    await wrapper.get('textarea').setValue('updated notes')
    const payload = await submit(wrapper)
    expect(payload.notes).toBe('updated notes')
    expect(payload.extra).not.toHaveProperty('quality_tier')
  })

  it.each(['apikey', 'oauth'])('preserves unchanged quality fields for %s edits', async (type) => {
    const wrapper = mountModal({
      platform: 'openai', type, quality_tier: 'strong',
      extra: { quality_tier: 'strong', qos_resource_pool: 'shared-fixture', qos_upstream_kind: 'direct' }
    })
    await wrapper.get('textarea').setValue('updated notes')
    const payload = await submit(wrapper)
    expect(payload.extra).not.toHaveProperty('quality_tier')
    expect(payload.extra).not.toHaveProperty('qos_resource_pool')
    expect(payload.extra).not.toHaveProperty('qos_upstream_kind')
  })

  it('still allows explicitly changing Strong to Unknown', async () => {
    const wrapper = mountModal({
      platform: 'openai', quality_tier: 'strong', extra: { quality_tier: 'strong' }
    })
    wrapper.findComponent({ name: 'AccountQualityFields' }).vm.$emit('update:tier', 'unknown')
    const payload = await submit(wrapper)
    expect(payload.extra).toHaveProperty('quality_tier', 'unknown')
  })
})
