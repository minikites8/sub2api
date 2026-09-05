import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminUser } from '@/types'
import UserRechargeDiscountCouponModal from '../UserRechargeDiscountCouponModal.vue'

const { issueRechargeDiscountCoupon, listRechargeDiscountCoupons, showSuccess, showError } = vi.hoisted(() => ({
  issueRechargeDiscountCoupon: vi.fn(),
  listRechargeDiscountCoupons: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { issueRechargeDiscountCoupon, listRechargeDiscountCoupons } },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const testUser = {
  id: 42,
  email: 'coupon@example.com',
  username: 'coupon',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-09-05T00:00:00Z',
  updated_at: '2026-09-05T00:00:00Z',
  notes: '',
} as AdminUser

const mountModal = () => mount(UserRechargeDiscountCouponModal, {
  props: { show: true, user: testUser },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>',
      },
      Icon: true,
    },
  },
})

describe('UserRechargeDiscountCouponModal', () => {
  beforeEach(() => {
    issueRechargeDiscountCoupon.mockReset().mockResolvedValue({ id: 1 })
    listRechargeDiscountCoupons.mockReset().mockResolvedValue([])
    showSuccess.mockReset()
    showError.mockReset()
  })

  it('submits threshold, discount rate, uses, and notes', async () => {
    const wrapper = mountModal()
    await wrapper.get('#coupon-min-amount').setValue('200')
    await wrapper.get('#coupon-discount-rate').setValue('8.5')
    await wrapper.get('#coupon-total-uses').setValue('3')
    await wrapper.get('#coupon-notes').setValue(' retention ')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(issueRechargeDiscountCoupon).toHaveBeenCalledWith(42, {
      min_recharge_amount: 200,
      discount_rate: 8.5,
      total_uses: 3,
      notes: 'retention',
    })
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(listRechargeDiscountCoupons).toHaveBeenCalledTimes(2)
  })

  it('keeps submit disabled for an invalid discount rate', async () => {
    const wrapper = mountModal()
    await wrapper.get('#coupon-discount-rate').setValue('10')

    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('shows issued coupons with usage and derived status', async () => {
    listRechargeDiscountCoupons.mockResolvedValue([
      {
        id: 7,
        user_id: 42,
        min_recharge_amount: 0,
        discount_percent: 85,
        total_uses: 0,
        used_count: 1,
        remaining_uses: 0,
        status: 'active',
        created_by: 99,
        notes: 'retention',
        created_at: '2026-09-05T00:00:00Z',
        updated_at: '2026-09-05T00:00:00Z',
        source_type: 'promo_code',
        source_id: 17,
        source_code: 'PARTNER85',
      },
      {
        id: 8,
        user_id: 42,
        min_recharge_amount: 100,
        discount_percent: 90,
        total_uses: 1,
        used_count: 1,
        remaining_uses: 0,
        status: 'active',
        created_by: 99,
        notes: '',
        created_at: '2026-09-04T00:00:00Z',
        updated_at: '2026-09-04T00:00:00Z',
        source_type: 'admin',
      },
    ])

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.findAll('[data-test="coupon-item"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('retention')
    expect(wrapper.text()).toContain('admin.users.rechargeCoupon.unlimitedUsage')
    expect(wrapper.text()).toContain('admin.users.rechargeCoupon.couponRuleNoThreshold')
    expect(wrapper.text()).toContain('admin.users.rechargeCoupon.status.active')
    expect(wrapper.text()).toContain('admin.users.rechargeCoupon.status.exhausted')
    expect(wrapper.text()).toContain('admin.users.rechargeCoupon.sourcePromoCode')
  })
})
