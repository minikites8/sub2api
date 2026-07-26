import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EditUsernameModal from '@/components/user/profile/EditUsernameModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const { updateProfileMock, showSuccessMock, showErrorMock, authState } = vi.hoisted(() => ({
  updateProfileMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  authState: { user: null as Record<string, unknown> | null }
}))

vi.mock('@/api', () => ({
  userAPI: {
    updateProfile: updateProfileMock
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: showSuccessMock,
    showError: showErrorMock
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('EditUsernameModal', () => {
  beforeEach(() => {
    updateProfileMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    authState.user = null
  })

  it('pre-fills the input with the current username', () => {
    const wrapper = mount(EditUsernameModal, {
      props: { show: true, currentUsername: 'alice' },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    expect((wrapper.get('#edit-username-input').element as HTMLInputElement).value).toBe('alice')
  })

  it('shows a toast and does not call the API when the username is blank', async () => {
    const wrapper = mount(EditUsernameModal, {
      props: { show: true, currentUsername: 'alice' },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await wrapper.get('#edit-username-input').setValue('   ')
    await wrapper.get('button.btn-primary').trigger('click')

    expect(updateProfileMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('profile.usernameRequired')
  })

  it('saves the new username and emits close on success', async () => {
    updateProfileMock.mockResolvedValue({ id: 1, username: 'bob' })
    const wrapper = mount(EditUsernameModal, {
      props: { show: true, currentUsername: 'alice' },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await wrapper.get('#edit-username-input').setValue('bob')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateProfileMock).toHaveBeenCalledWith({ username: 'bob' })
    expect(authState.user).toEqual({ id: 1, username: 'bob' })
    expect(showSuccessMock).toHaveBeenCalledWith('profile.updateSuccess')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('shows an error toast and does not close when the API call fails', async () => {
    updateProfileMock.mockRejectedValue({ response: { data: { detail: 'boom' } } })
    const wrapper = mount(EditUsernameModal, {
      props: { show: true, currentUsername: 'alice' },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await wrapper.get('#edit-username-input').setValue('bob')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeFalsy()
  })

  it('emits close without saving when cancel is clicked', async () => {
    const wrapper = mount(EditUsernameModal, {
      props: { show: true, currentUsername: 'alice' },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await wrapper.get('button.btn-secondary').trigger('click')

    expect(updateProfileMock).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})
