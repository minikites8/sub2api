import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DiscountCouponImportModal from '../DiscountCouponImportModal.vue'

const { importDiscountCoupons, readCouponExcel, downloadCouponTemplate, showSuccess, showError } = vi.hoisted(() => ({
  importDiscountCoupons: vi.fn(), readCouponExcel: vi.fn(), downloadCouponTemplate: vi.fn(), showSuccess: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { importDiscountCoupons } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('../discountCouponExcel', () => ({
  readCouponExcel, downloadCouponTemplate, CouponExcelError: class extends Error {},
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => true }) }))

const row = { row_number: 2, user_id: 42, coupon_type: 'subscription', min_amount: 20, discount_rate: 8, total_uses: 3 }
const preview = { valid: true, issued_count: 0, rows: [{ row_number: 2, user_id: 42, email: 'user@example.com' }] }
function mountModal() {
  return mount(DiscountCouponImportModal, {
    props: { show: true },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      Icon: true,
    } },
  })
}
async function selectFile(wrapper: ReturnType<typeof mountModal>) {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['test'], 'coupons.xlsx')] })
  await input.trigger('change')
  await flushPromises()
}

describe('DiscountCouponImportModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    readCouponExcel.mockResolvedValue([{ data: row }])
    importDiscountCoupons.mockResolvedValue(preview)
    downloadCouponTemplate.mockResolvedValue(undefined)
  })

  it('validates without issuing and enables issuance after preview', async () => {
    const wrapper = mountModal()
    expect(wrapper.get('[data-test="issue-import"]').attributes('disabled')).toBeDefined()
    await selectFile(wrapper)
    expect(importDiscountCoupons).toHaveBeenCalledWith([row], true, expect.stringMatching(/^coupon-import-/))
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.get('[data-test="issue-import"]').attributes('disabled')).toBeUndefined()
    importDiscountCoupons.mockResolvedValue({ ...preview, issued_count: 1, rows: [{ row_number: 2, coupon_id: 7 }] })
    await wrapper.get('[data-test="issue-import"]').trigger('click')
    await flushPromises()
    expect(importDiscountCoupons.mock.calls[1]).toEqual([[{ ...row, email: 'user@example.com' }], false, importDiscountCoupons.mock.calls[0][2]])
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.get('[data-test="issue-import"]').attributes('disabled')).toBeDefined()
  })

  it('shows local row errors and waits for corrected data', async () => {
    readCouponExcel.mockResolvedValue([{ data: row, errorCode: 'INVALID_COUPON_DISCOUNT' }])
    const wrapper = mountModal()
    await selectFile(wrapper)
    expect(importDiscountCoupons).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.users.couponImport.errors.INVALID_COUPON_DISCOUNT')
    expect(wrapper.get('[data-test="issue-import"]').attributes('disabled')).toBeDefined()
  })

  it('shows server user errors and keeps the batch pending', async () => {
    importDiscountCoupons.mockResolvedValue({ valid: false, issued_count: 0, rows: [{ row_number: 2, error_code: 'USER_NOT_FOUND', error: 'user not found' }] })
    const wrapper = mountModal()
    await selectFile(wrapper)
    expect(wrapper.text()).toContain('admin.users.couponImport.errors.USER_NOT_FOUND')
    expect(wrapper.get('[data-test="issue-import"]').attributes('disabled')).toBeDefined()
  })

  it('reuses the request key after a timeout and locks file selection during issuance', async () => {
    const wrapper = mountModal()
    await selectFile(wrapper)
    let fail!: (reason: Error) => void
    importDiscountCoupons.mockReturnValueOnce(new Promise((_, reject) => { fail = reject }))
    await wrapper.get('[data-test="issue-import"]').trigger('click')
    expect(wrapper.get('input[type="file"]').attributes('disabled')).toBeDefined()
    fail(new Error('timeout'))
    await flushPromises()
    expect(wrapper.text()).toContain('admin.users.couponImport.issueFailed')
    await wrapper.get('[data-test="issue-import"]').trigger('click')
    await flushPromises()
    expect(importDiscountCoupons.mock.calls[1][2]).toBe(importDiscountCoupons.mock.calls[2][2])
  })

  it('downloads both coupon templates', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="template-recharge"]').trigger('click')
    await wrapper.get('[data-test="template-subscription"]').trigger('click')
    expect(downloadCouponTemplate.mock.calls.map(call => call[0])).toEqual(['recharge', 'subscription'])
  })
})
