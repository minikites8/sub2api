import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const {
  generateKiroIDCAuthUrlMock, reimportCredentialsMock, generateAuthUrlMock,
  exchangeCodeMock, refreshOpenAITokenMock, applyOAuthCredentialsMock,
} = vi.hoisted(() => ({
  generateKiroIDCAuthUrlMock: vi.fn(),
  reimportCredentialsMock: vi.fn(),
  generateAuthUrlMock: vi.fn(),
  exchangeCodeMock: vi.fn(),
  refreshOpenAITokenMock: vi.fn(),
  applyOAuthCredentialsMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    kiro: {
      generateIDCAuthUrl: generateKiroIDCAuthUrlMock,
    },
    accounts: {
      applyOAuthCredentials: applyOAuthCredentialsMock,
      reimportCredentials: reimportCredentialsMock,
      generateAuthUrl: generateAuthUrlMock,
      exchangeCode: exchangeCodeMock,
      refreshOpenAIToken: refreshOpenAITokenMock,
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import ReAuthAccountModal from '../ReAuthAccountModal.vue'
import { OPENAI_EXCEL_OAUTH_CLIENT_ID } from '@/composables/useOpenAIOAuth'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: {
    platform: { type: String, default: '' },
    excelOauth: { type: Boolean, default: false },
    error: { type: String, default: '' },
  },
  emits: ['generate-url', 'validate-refresh-token'],
  setup(_, { expose }) {
    expose({
      authCode: ref('new-code'),
      oauthState: ref('test-state'),
      inputMethod: ref('manual'),
      reset: vi.fn(),
    })
  },
  template: '<button data-testid="reauth-generate-url" @click="$emit(\'generate-url\')">generate</button>',
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value, null)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

function buildKiroIDCAccount() {
  return {
    id: 12,
    name: 'Kiro IDC',
    platform: 'kiro',
    type: 'oauth',
    credentials: {
      auth_method: 'idc',
      provider: 'Enterprise',
      start_url: 'https://view.awsapps.com/start',
      region: 'eu-central-1',
      refresh_token: 'refresh-token',
    },
    extra: {},
    proxy_id: null,
  } as any
}

function buildGrokAccount() {
  return {
    id: 21,
    name: 'Grok OAuth',
    platform: 'grok',
    type: 'oauth',
    credentials: { refresh_token: 'refresh-token' },
    extra: {},
    proxy_id: null,
  } as any
}

function mountModal(account: any = buildKiroIDCAccount()) {
  return mount(ReAuthAccountModal, {
    props: {
      show: false,
      account,
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
      },
    },
  })
}

describe('ReAuthAccountModal Kiro regions', () => {
  beforeEach(() => {
    generateKiroIDCAuthUrlMock.mockReset().mockResolvedValue({
      auth_url: 'https://kiro.example/auth',
      session_id: 'session-id',
      state: 'state',
    })
  })

  it('rehydrates Kiro IDC region with a code-only select and submits the selected code', async () => {
    const wrapper = mountModal()

    await wrapper.setProps({ show: true })
    await flushPromises()

    const regionSelect = wrapper.get<HTMLSelectElement>('[data-testid="reauth-kiro-idc-region-select"]')
    expect(regionSelect.element.value).toBe('eu-central-1')
    expect(regionSelect.find('option[value="us-east-1"]').exists()).toBe(true)
    expect(regionSelect.find('option[value="us-east-1"]').text()).toBe('us-east-1')

    await regionSelect.setValue('eu-west-1')
    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()

    expect(generateKiroIDCAuthUrlMock).toHaveBeenCalledWith({
      proxy_id: undefined,
      start_url: 'https://view.awsapps.com/start',
      region: 'eu-west-1',
    })
  })
})

describe('ReAuthAccountModal credential import', () => {
  it('uses the unified reauthorize dialog for non-OAuth accounts', async () => {
    const account = {
      id: 44,
      name: 'API key account',
      platform: 'anthropic',
      type: 'apikey',
      credentials: { api_key: 'old', model_mapping: { claude: 'claude-sonnet' } },
      extra: {},
      proxy_id: null,
    }
    const payload = {
      type: 'sub2api-data',
      version: 1,
      proxies: [],
      accounts: [{ name: 'exported', platform: 'anthropic', type: 'apikey', credentials: { api_key: 'new' } }],
    }
    reimportCredentialsMock.mockResolvedValue({ ...account, credentials: { api_key: 'new' } })
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await flushPromises()

    const input = wrapper.get('input[type="file"]')
    const file = new File([JSON.stringify(payload)], 'sub2api.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve(JSON.stringify(payload)) })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(reimportCredentialsMock).toHaveBeenCalledWith(44, payload)
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
  })
})

describe('ReAuthAccountModal platform routing', () => {
  // 回归:Grok 账号曾因 oauthPlatform 缺少分支回落到 anthropic,
  // 导致弹窗显示 Claude 文案且 callback URL 自动提取 code/state 失效。
  it('passes platform="grok" to the OAuth flow for Grok accounts', async () => {
    const wrapper = mountModal(buildGrokAccount())

    await wrapper.setProps({ show: true })
    await flushPromises()

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    expect(flow.props('platform')).toBe('grok')
    expect(wrapper.text()).toContain('admin.accounts.grokAccount')
  })
})

describe('ReAuthAccountModal BPS reauthorization', () => {
  const excelClientID = OPENAI_EXCEL_OAUTH_CLIENT_ID
  const codexClientID = 'app_EMoamEEZ73f0CkXaXp7hrann'
  const buildOpenAIAccount = (clientID = excelClientID) => ({
    id: 52,
    name: 'BPS OAuth',
    platform: 'openai',
    type: 'oauth',
    credentials: { client_id: clientID, model_mapping: { 'gpt-5.6-sol': 'gpt-5.6-sol' } },
    extra: { openai_excel_bps: true, openai_excel_bps_models: ['gpt-5.6-sol'] },
    proxy_id: 27,
  })
  const tokens = (clientID: string | undefined = excelClientID) => ({
    access_token: 'new-access-token', refresh_token: 'new-refresh-token',
    id_token: 'new-id-token', client_id: clientID, expires_at: 1900000000,
    email: 'bps@example.com',
  })

  beforeEach(() => {
    generateAuthUrlMock.mockReset().mockResolvedValue({
      auth_url: 'https://auth.openai.com/authorize?state=test-state', session_id: 'excel-session',
    })
    exchangeCodeMock.mockReset().mockResolvedValue(tokens())
    refreshOpenAITokenMock.mockReset().mockResolvedValue(tokens())
    applyOAuthCredentialsMock.mockReset().mockResolvedValue(buildOpenAIAccount())
  })

  it('uses the Excel authorization flow even after BPS was automatically disabled', async () => {
    const account = buildOpenAIAccount(' ' + excelClientID + ' ')
    account.extra.openai_excel_bps = false
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    expect(wrapper.getComponent(OAuthAuthorizationFlowStub).props('excelOauth')).toBe(true)

    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()
    expect(generateAuthUrlMock).toHaveBeenCalledWith('/admin/openai/generate-auth-url', {
      proxy_id: 27, oauth_client: 'excel',
    })
  })

  it('exchanges the Excel callback and applies credentials to the existing account', async () => {
    const account = buildOpenAIAccount()
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(exchangeCodeMock).toHaveBeenCalledWith('/admin/openai/exchange-code', {
      code: 'new-code', session_id: 'excel-session', state: 'test-state', proxy_id: 27,
    })
    expect(applyOAuthCredentialsMock).toHaveBeenCalledWith(52, {
      type: 'oauth',
      credentials: expect.objectContaining({
        ...tokens(), model_mapping: account.credentials.model_mapping,
      }),
      extra: { email: 'bps@example.com' },
    })
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(account.extra).toEqual({ openai_excel_bps: true, openai_excel_bps_models: ['gpt-5.6-sol'] })
  })

  it('validates a BPS refresh token with the Excel client before applying it', async () => {
    const wrapper = mountModal(buildOpenAIAccount())
    await wrapper.setProps({ show: true })
    wrapper.getComponent(OAuthAuthorizationFlowStub).vm.$emit('validate-refresh-token', ' excel-rt ')
    await flushPromises()

    expect(refreshOpenAITokenMock).toHaveBeenCalledWith('excel-rt', 27, '/admin/openai/refresh-token', excelClientID)
    expect(applyOAuthCredentialsMock).toHaveBeenCalledWith(52, {
      type: 'oauth', credentials: expect.objectContaining(tokens()), extra: { email: 'bps@example.com' },
    })
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
  })

  it.each([
    ['manual', codexClientID], ['refresh_token', codexClientID],
    ['manual', undefined], ['refresh_token', undefined],
  ])('rejects an incompatible client %s / %s in the BPS flow', async (method, clientID) => {
    const incompatibleTokens = { ...tokens(), client_id: clientID }
    exchangeCodeMock.mockResolvedValue(incompatibleTokens)
    refreshOpenAITokenMock.mockResolvedValue(incompatibleTokens)
    const wrapper = mountModal(buildOpenAIAccount())
    await wrapper.setProps({ show: true })
    if (method === 'manual') {
      await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
      await flushPromises()
      await wrapper.get('button.btn-primary').trigger('click')
    } else {
      wrapper.getComponent(OAuthAuthorizationFlowStub).vm.$emit('validate-refresh-token', 'excel-rt')
    }
    await flushPromises()

    expect(applyOAuthCredentialsMock).not.toHaveBeenCalled()
    expect(wrapper.emitted('reauthorized')).toBeUndefined()
    expect(wrapper.getComponent(OAuthAuthorizationFlowStub).props('error')).toBe('admin.accounts.oauth.openai.excelSessionMismatch')
  })

  it('retains Codex authorization for a Codex account with BPS enabled', async () => {
    refreshOpenAITokenMock.mockResolvedValue(tokens(codexClientID))
    const wrapper = mountModal(buildOpenAIAccount(codexClientID))
    await wrapper.setProps({ show: true })
    expect(wrapper.getComponent(OAuthAuthorizationFlowStub).props('excelOauth')).toBe(false)
    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()
    expect(generateAuthUrlMock).toHaveBeenCalledWith('/admin/openai/generate-auth-url', { proxy_id: 27 })

    wrapper.getComponent(OAuthAuthorizationFlowStub).vm.$emit('validate-refresh-token', 'codex-rt')
    await flushPromises()
    expect(refreshOpenAITokenMock).toHaveBeenCalledWith('codex-rt', 27, '/admin/openai/refresh-token', undefined)
    expect(applyOAuthCredentialsMock).toHaveBeenCalledWith(52, expect.objectContaining({
      credentials: expect.objectContaining({ client_id: codexClientID }),
    }))
  })
})
