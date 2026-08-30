import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ReimportCredentialsModal from '@/components/admin/account/ReimportCredentialsModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      reimportCredentials: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const account = {
  id: 21,
  name: 'existing',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  starts_at: '',
  daily_usage_usd: 0,
  weekly_usage_usd: 0,
  monthly_usage_usd: 0,
  daily_window_start: null,
  weekly_window_start: null,
  monthly_window_start: null,
  created_at: '',
  updated_at: '',
  expires_at: null
} as any

const makeJsonFile = (content: string) => {
  const file = new File([content], 'sub2api.json', { type: 'application/json' })
  Object.defineProperty(file, 'text', { value: () => Promise.resolve(content) })
  return file
}

const setInputFiles = (element: Element, files: File[]) => {
  Object.defineProperty(element, 'files', { value: files, configurable: true })
}

const mountModal = () => mount(ReimportCredentialsModal, {
  props: { show: true, account },
  global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
})

describe('ReimportCredentialsModal', () => {
  beforeEach(async () => {
    showError.mockReset()
    showSuccess.mockReset()
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.reimportCredentials).mockReset()
  })

  it('requires one account and forwards only the selected export payload', async () => {
    const { adminAPI } = await import('@/api/admin')
    const updated = { ...account, credentials: { access_token: 'new' } }
    vi.mocked(adminAPI.accounts.reimportCredentials).mockResolvedValue(updated)
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    const payload = {
      type: 'sub2api-data',
      version: 1,
      proxies: [],
      accounts: [{ name: 'exported', platform: 'openai', type: 'oauth', credentials: { access_token: 'new' } }]
    }
    setInputFiles(input.element, [makeJsonFile(JSON.stringify(payload))])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.reimportCredentials).toHaveBeenCalledWith(21, payload)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.reimportCredentialsSuccess')
    expect(wrapper.emitted('reimported')).toHaveLength(1)
  })

  it('rejects a file containing multiple accounts before calling the API', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [makeJsonFile(JSON.stringify({
      proxies: [],
      accounts: [
        { name: 'one', platform: 'openai', type: 'oauth', credentials: { access_token: '1' } },
        { name: 'two', platform: 'openai', type: 'oauth', credentials: { access_token: '2' } }
      ]
    }))])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.reimportCredentials).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.reimportCredentialsSingleAccount')
  })
})
