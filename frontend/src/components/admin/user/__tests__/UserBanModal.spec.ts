import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminGroup, AdminUser } from '@/types'
import UserBanModal from '../UserBanModal.vue'

const { banUser, unbanUser, banGroup, unbanGroup, showSuccess, showError } = vi.hoisted(() => ({
  banUser: vi.fn(),
  unbanUser: vi.fn(),
  banGroup: vi.fn(),
  unbanGroup: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { banUser, unbanUser, banGroup, unbanGroup }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const user = (overrides: Partial<AdminUser> = {}): AdminUser => ({
  id: 42,
  email: 'user@example.com',
  username: 'user',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-09-04T00:00:00Z',
  updated_at: '2026-09-04T00:00:00Z',
  notes: '',
  ...overrides
})

const groups = [
  { id: 7, name: 'Claude', status: 'active' },
  { id: 9, name: 'Codex', status: 'active' }
] as AdminGroup[]

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `
    <select
      data-test="group-select-stub"
      :value="modelValue ?? ''"
      @change="$emit('update:modelValue', Number($event.target.value))"
    >
      <option value=""></option>
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
}

const mountModal = (targetUser: AdminUser) => mount(UserBanModal, {
  props: { show: true, user: targetUser, groups },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      Select: SelectStub,
      Icon: true
    }
  }
})

describe('UserBanModal', () => {
  beforeEach(() => {
    banUser.mockReset().mockResolvedValue(user({ status: 'disabled' }))
    unbanUser.mockReset().mockResolvedValue(user())
    banGroup.mockReset().mockResolvedValue(user({ banned_group_ids: [7] }))
    unbanGroup.mockReset().mockResolvedValue(user())
    showSuccess.mockReset()
    showError.mockReset()
  })

  it('applies a permanent user ban', async () => {
    const wrapper = mountModal(user())

    await wrapper.get('[data-test="submit-ban"]').trigger('click')
    await flushPromises()

    expect(banUser).toHaveBeenCalledWith(42, { duration_hours: 0 })
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('converts a temporary group ban from days to hours', async () => {
    const wrapper = mountModal(user())

    await wrapper.get('[data-test="ban-scope-group"]').trigger('click')
    await wrapper.get('[data-test="ban-group-select"]').setValue('7')
    await wrapper.get('[data-test="ban-duration-temporary"]').trigger('click')
    await wrapper.get('[data-test="ban-duration-value"]').setValue('3')
    await wrapper.get('[data-test="ban-duration-unit"]').setValue('days')
    await wrapper.get('[data-test="submit-ban"]').trigger('click')
    await flushPromises()

    expect(banGroup).toHaveBeenCalledWith(42, 7, { duration_hours: 72 })
  })

  it('removes account and group bans independently', async () => {
    const wrapper = mountModal(user({
      status: 'disabled',
      disabled_until: null,
      banned_group_ids: [9]
    }))

    await wrapper.get('[data-test="unban-user"]').trigger('click')
    await flushPromises()
    expect(unbanUser).toHaveBeenCalledWith(42)

    const groupWrapper = mountModal(user({ banned_group_ids: [9] }))
    await groupWrapper.get('[data-test="ban-scope-group"]').trigger('click')
    await groupWrapper.get('[data-test="unban-group-9"]').trigger('click')
    await flushPromises()
    expect(unbanGroup).toHaveBeenCalledWith(42, 9)
  })
})
