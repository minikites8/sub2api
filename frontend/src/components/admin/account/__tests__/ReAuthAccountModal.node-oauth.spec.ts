import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Account } from '@/types'

const {
  getAccountByIdMock,
  applyOAuthCredentialsMock,
  listNodeLeaseNodesMock,
  createNodeLoginTaskMock,
  listNodeLoginTasksMock,
  submitNodeLoginTaskCallbackMock,
} = vi.hoisted(() => ({
  getAccountByIdMock: vi.fn(),
  applyOAuthCredentialsMock: vi.fn(),
  listNodeLeaseNodesMock: vi.fn(),
  createNodeLoginTaskMock: vi.fn(),
  listNodeLoginTasksMock: vi.fn(),
  submitNodeLoginTaskCallbackMock: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getById: getAccountByIdMock,
      applyOAuthCredentials: applyOAuthCredentialsMock,
      exchangeCode: vi.fn(),
    },
    nodeLeases: {
      listNodes: listNodeLeaseNodesMock,
      createLoginTask: createNodeLoginTaskMock,
      listLoginTasks: listNodeLoginTasksMock,
      submitLoginTaskCallback: submitNodeLoginTaskCallbackMock,
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

const oauthStubState = {
  authUrl: { value: '' },
  sessionId: { value: '' },
  loading: { value: false },
  error: { value: '' },
  oauthState: { value: '' },
  state: { value: '' },
  externalIdpStage: { value: '' },
  resetState: vi.fn(),
  generateAuthUrl: vi.fn(),
  generateIDCAuthUrl: vi.fn(),
  exchangeAuthCode: vi.fn(),
  importToken: vi.fn(),
  buildCredentials: vi.fn((tokenInfo: Record<string, unknown>) => tokenInfo),
  buildExtraInfo: vi.fn(() => ({})),
}

vi.mock('@/composables/useOpenAIOAuth', () => ({
  useOpenAIOAuth: () => oauthStubState,
}))

vi.mock('@/composables/useGrokOAuth', () => ({
  useGrokOAuth: () => oauthStubState,
}))

vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => oauthStubState,
}))

vi.mock('@/composables/useAntigravityOAuth', () => ({
  useAntigravityOAuth: () => oauthStubState,
}))

vi.mock('@/composables/useKiroOAuth', () => ({
  useKiroOAuth: () => oauthStubState,
}))

vi.mock('@/composables/useAccountOAuth', () => ({
  useAccountOAuth: () => oauthStubState,
}))

import ReAuthAccountModal from '../ReAuthAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: {
    authUrl: String,
    sessionId: String,
  },
  emits: ['generate-url', 'cookie-auth'],
  data: () => ({
    authCode: '',
    oauthState: '',
    oauthCallbackPath: '',
    oauthLoginOption: '',
    projectId: '',
    sessionKey: '',
    inputMethod: 'manual',
  }),
  template: `
    <div>
      <span data-testid="reauth-auth-url">{{ authUrl }}</span>
      <span data-testid="reauth-session-id">{{ sessionId }}</span>
      <button data-testid="reauth-generate-url" @click="$emit('generate-url')">generate</button>
    </div>
  `,
})

const account = {
  id: 77,
  name: 'OpenAI old account',
  platform: 'openai',
  type: 'oauth',
  credentials: {
    model_mapping: { mode: 'allow', models: ['gpt-5'] },
    access_token: 'masked-old-access',
    node_oauth_pending: true,
  },
  proxy_id: 9,
  group_ids: [1, 2],
  concurrency: 5,
  priority: 8,
  status: 'error',
  error_message: 'token expired',
} as Account

const node = {
  node_id: 'sub2api-node-ip-0',
  region: 'jp',
  status: 'online',
  inflight_requests: 0,
  lease_remaining: 0,
  registered_at: '2026-07-18T00:00:00Z',
  updated_at: '2026-07-18T00:00:00Z',
}

const waitingTask = {
  id: 'ql_account_task_reauth',
  account_id: 77,
  name: 'OpenAI old account',
  platform: 'openai',
  type: 'oauth',
  assigned_node_id: 'sub2api-node-ip-0',
  login_payload: {
    auth_url: 'https://auth.openai.example/start',
    session_id: 'session-1',
    state: 'state-1',
    redirect_uri: 'http://localhost:1455/auth/callback',
  },
  group_ids: [1, 2],
  concurrency: 5,
  priority: 8,
  status: 'waiting_callback',
  created_at: '2026-07-18T00:00:00Z',
  updated_at: '2026-07-18T00:00:00Z',
}

function mountModal() {
  return mount(ReAuthAccountModal, {
    props: { show: true, account },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        Icon: true,
      },
    },
  })
}

describe('ReAuthAccountModal node OAuth', () => {
  beforeEach(() => {
    sessionStorage.clear()
    getAccountByIdMock.mockReset().mockResolvedValue({ ...account, status: 'active', error_message: null })
    applyOAuthCredentialsMock.mockReset()
    listNodeLeaseNodesMock.mockReset().mockResolvedValue([node])
    createNodeLoginTaskMock.mockReset().mockResolvedValue({
      ...waitingTask,
      status: 'pending',
      login_payload: {},
    })
    listNodeLoginTasksMock.mockReset()
    submitNodeLoginTaskCallbackMock.mockReset().mockResolvedValue({
      ...waitingTask,
      status: 'callback_ready',
    })
  })

  it('runs re-authorization through a selected node', async () => {
    const completedTask = {
      ...waitingTask,
      status: 'completed',
      account: {
        id: 77,
        name: 'OpenAI old account',
        platform: 'openai',
        type: 'oauth',
        credentials: { access_token: 'fresh-access' },
        status: 'active',
        schedulable: true,
        concurrency: 5,
        priority: 8,
        updated_at: '2026-07-18T00:00:01Z',
      },
    }
    listNodeLoginTasksMock
      .mockResolvedValueOnce([waitingTask])
      .mockResolvedValueOnce([completedTask])

    const wrapper = mountModal()
    await wrapper.get('[data-testid="reauth-node-oauth-enabled"]').setValue(true)
    await flushPromises()
    await wrapper.get('[data-testid="reauth-node-oauth-load-nodes"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="reauth-generate-url"]').trigger('click')
    await flushPromises()

    expect(createNodeLoginTaskMock).toHaveBeenCalledTimes(1)
    expect(createNodeLoginTaskMock.mock.calls[0]?.[0]).toMatchObject({
      account_id: 77,
      name: 'OpenAI old account',
      platform: 'openai',
      type: 'oauth',
      assigned_node_id: 'sub2api-node-ip-0',
      metadata: { source: 'account_reauth_modal' },
      group_ids: [1, 2],
      concurrency: 5,
      priority: 8,
      login_payload: {
        credential_overrides: {
          model_mapping: { mode: 'allow', models: ['gpt-5'] },
        },
        proxy_id: 9,
      },
    })
    expect(createNodeLoginTaskMock.mock.calls[0]?.[0].login_payload.credential_overrides).not.toHaveProperty('access_token')
    expect(createNodeLoginTaskMock.mock.calls[0]?.[0].login_payload.credential_overrides).not.toHaveProperty('node_oauth_pending')
    expect(createNodeLoginTaskMock.mock.calls[0]?.[1]).toBeUndefined()
    expect(wrapper.get('[data-testid="reauth-auth-url"]').text()).toBe('https://auth.openai.example/start')

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    ;(flow.vm as any).authCode = 'callback-code'
    await nextTick()

    const completeButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.oauth.completeAuth'))
    expect(completeButton).toBeDefined()
    await completeButton?.trigger('click')
    await flushPromises()

    expect(submitNodeLoginTaskCallbackMock).toHaveBeenCalledWith(
      'ql_account_task_reauth',
      {
        code: 'callback-code',
        state: 'state-1',
        session_id: 'session-1',
        redirect_uri: 'http://localhost:1455/auth/callback',
        proxy_id: 9,
      }
    )
    expect(getAccountByIdMock).toHaveBeenCalledWith(77)
    expect(wrapper.emitted('reauthorized')?.[0]?.[0]).toMatchObject({
      id: 77,
      status: 'active',
      error_message: null,
    })
  })
})
