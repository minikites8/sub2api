import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import type { LocaleMessageObject, VueMessageType } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'
import ProfileCouponsCard from '../ProfileCouponsCard.vue'
import type { UserDiscountCoupon } from '@/types/payment'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const getMyCoupons = vi.hoisted(() => vi.fn())
vi.mock('@/api/payment', () => ({ paymentAPI: { getMyCoupons } }))

function coupon(overrides: Partial<UserDiscountCoupon> = {}): UserDiscountCoupon {
  return {
    id: 1, coupon_type: 'recharge', min_amount: 100, discount_percent: 80,
    total_uses: 3, used_count: 1, remaining_uses: 2, status: 'available',
    created_at: '2026-10-06T00:00:00Z', ...overrides,
  }
}

// Component tests use the runtime-only i18n build with compiled message functions.
function compileMessages(messages: Record<string, unknown>): LocaleMessageObject<VueMessageType> {
  return Object.fromEntries(Object.entries(messages).map(([key, value]) => [
    key,
    typeof value === 'string'
      ? new Function(`return ${baseCompile(value, { mode: 'arrow' }).code}`)()
      : compileMessages(value as Record<string, unknown>),
  ]))
}

const messages = {
  zh: compileMessages({ common: zh.common, profile: { coupons: zh.profile.coupons } }),
  en: compileMessages({ common: en.common, profile: { coupons: en.profile.coupons } }),
}

function mountCard(locale = 'zh') {
  return mount(ProfileCouponsCard, {
    global: {
      plugins: [createI18n({ legacy: false, locale, messages })],
      stubs: { RouterLink: RouterLinkStub },
    },
  })
}

describe('ProfileCouponsCard', () => {
  beforeEach(() => getMyCoupons.mockReset())

  it('shows recharge and subscription coupons with rules, remaining uses, and checkout links', async () => {
    getMyCoupons.mockResolvedValue({ data: [coupon(), coupon({ coupon_type: 'subscription', min_amount: 20, discount_percent: 85 })] })
    const wrapper = mountCard()
    expect(wrapper.text()).toContain('加载中')
    await flushPromises()
    expect(getMyCoupons).toHaveBeenCalledTimes(1)
    const items = wrapper.findAll('[data-testid="profile-coupon"]')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('充值优惠券')
    expect(items[0].text()).toContain('8 折')
    expect(items[0].text()).toContain('100.00')
    expect(items[0].text()).toContain('2 次')
    expect(items[0].text()).toContain('1 / 3')
    expect(items[1].text()).toContain('订阅优惠券')
    expect(items[1].text()).toContain('8.5 折')
    expect(items[1].text()).toContain('套餐原价合计满 $20.00 可用')
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links[0].props('to')).toEqual({ path: '/purchase', query: { tab: 'recharge' } })
    expect(links[1].props('to')).toEqual({ path: '/purchase', query: { tab: 'subscription' } })
    wrapper.unmount()
  })

  it('shows unlimited, exhausted, and inactive coupons with their current availability', async () => {
    getMyCoupons.mockResolvedValue({ data: [
      coupon({ total_uses: 0, used_count: 7, remaining_uses: 0 }),
      coupon({ id: 2, status: 'exhausted', remaining_uses: 0, used_count: 3 }),
      coupon({ id: 3, status: 'inactive' }),
    ] })
    const wrapper = mountCard()
    await flushPromises()
    const items = wrapper.findAll('[data-testid="profile-coupon"]')
    expect(items[0].get('[data-testid="coupon-remaining"]').text()).toBe('不限次数')
    expect(items[1].text()).toContain('已用完')
    expect(items[1].get('[data-testid="coupon-remaining"]').text()).toBe('0 次')
    expect(items[2].text()).toContain('已停用')
    expect(wrapper.findAllComponents(RouterLinkStub)).toHaveLength(1)
    wrapper.unmount()
  })

  it('handles an empty coupon collection', async () => {
    getMyCoupons.mockResolvedValue({ data: [] })
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('[data-testid="coupons-empty"]').text()).toBe('您当前暂无优惠券')
    wrapper.unmount()
  })

  it('retries failed requests and refreshes coupon usage', async () => {
    getMyCoupons.mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ data: [coupon()] })
      .mockResolvedValueOnce({ data: [coupon({ remaining_uses: 1, used_count: 2 })] })
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('优惠券加载失败')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="coupon-remaining"]').text()).toBe('2 次')
    await wrapper.get('[data-testid="refresh-coupons"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="coupon-remaining"]').text()).toBe('1 次')
    expect(getMyCoupons).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('explains the payment percentage in English', async () => {
    getMyCoupons.mockResolvedValue({ data: [coupon()] })
    const wrapper = mountCard('en')
    await flushPromises()
    expect(wrapper.text()).toContain('Pay 80%')
    expect(wrapper.text()).toContain('2 uses')
    wrapper.unmount()
  })
})
