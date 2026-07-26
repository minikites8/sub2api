import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import InvoiceView from '@/views/user/InvoiceView.vue'

const {
  getInvoiceableOrdersMock,
  listInvoicesMock,
  createInvoiceMock,
  cancelInvoiceMock,
  showSuccessMock,
  showErrorMock,
  routeState,
  routerReplace,
  authState
} = vi.hoisted(() => ({
  getInvoiceableOrdersMock: vi.fn(),
  listInvoicesMock: vi.fn(),
  createInvoiceMock: vi.fn(),
  cancelInvoiceMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  routeState: { path: '/invoices', query: {} as Record<string, unknown> },
  routerReplace: vi.fn(),
  authState: { user: { email: 'dev@example.com' } as Record<string, unknown> | null }
}))

vi.mock('@/api/invoice', () => ({
  default: {
    getInvoiceableOrders: getInvoiceableOrdersMock,
    listInvoices: listInvoicesMock,
    createInvoice: createInvoiceMock,
    cancelInvoice: cancelInvoiceMock
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: showSuccessMock, showError: showErrorMock })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({ replace: routerReplace })
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const STUBS = {
  AppLayout: { template: '<div><slot /></div>' },
  // BaseDialog teleports to body, which puts its content outside the wrapper.
  // Render it inline so the withdraw confirmation is reachable from the test.
  BaseDialog: {
    props: ['show'],
    template: '<div v-if="show" class="dialog-stub"><slot /><slot name="footer" /></div>'
  },
  Select: true
}

function mountView() {
  return mount(InvoiceView, { global: { stubs: STUBS } })
}

const ORDERS = [
  { order_id: 10, out_trade_no: 'TX-982734', description: 'Balance recharge', amount: 5, created_at: '2026-10-24T14:22:01Z' },
  { order_id: 11, out_trade_no: 'TX-982110', description: 'Balance recharge', amount: 12.5, created_at: '2026-10-15T18:44:22Z' }
]

async function fillBillingForm(wrapper: ReturnType<typeof mountView>) {
  await wrapper.get('#invoice-title-input').setValue('Acme Corp LLC')
  await wrapper.get('#invoice-tax-input').setValue('US123456789')
  await wrapper.get('#invoice-email-input').setValue('billing@example.com')
}

describe('InvoiceView', () => {
  beforeEach(() => {
    getInvoiceableOrdersMock.mockReset()
    listInvoicesMock.mockReset()
    createInvoiceMock.mockReset()
    cancelInvoiceMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    routerReplace.mockReset()
    routeState.path = '/invoices'
    routeState.query = {}
    authState.user = { email: 'dev@example.com' }
    getInvoiceableOrdersMock.mockResolvedValue(ORDERS)
    listInvoicesMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    createInvoiceMock.mockResolvedValue({ id: 1, invoice_no: 'INV-2026-7F3A9C' })
  })

  it('opens on the apply tab and lists the invoiceable orders', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('.invoice-table tbody tr')).toHaveLength(2)
    expect(wrapper.text()).toContain('TX-982734')
  })

  it('defaults the delivery email to the account email', async () => {
    const wrapper = mountView()
    await flushPromises()

    const email = wrapper.get('#invoice-email-input').element as HTMLInputElement
    expect(email.value).toBe('dev@example.com')
  })

  // 金额来自 USD,界面统一按 Credits 展示(1 Credit = 0.01 USD)。
  it('totals the selected orders in Credits', async () => {
    const wrapper = mountView()
    await flushPromises()

    const checkboxes = wrapper.findAll('.invoice-table tbody input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)

    // 5 + 12.5 USD = 1,750 Credits
    expect(wrapper.get('.invoice-summary-total').text()).toContain('1,750')
  })

  it('keeps submit disabled until orders and billing details are both present', async () => {
    const wrapper = mountView()
    await flushPromises()

    const submit = () => wrapper.get('.invoice-submit')
    expect(submit().attributes('disabled')).toBeDefined()

    await wrapper.findAll('.invoice-table tbody input[type="checkbox"]')[0].setValue(true)
    // 选了订单但还没填抬头,仍不可提交。
    expect(submit().attributes('disabled')).toBeDefined()

    await fillBillingForm(wrapper)
    expect(submit().attributes('disabled')).toBeUndefined()
  })

  // 企业发票缺税号后端会拒,前端要提前挡住而不是提交后才报错。
  it('requires a tax id for a company invoice but not for an individual', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAll('.invoice-table tbody input[type="checkbox"]')[0].setValue(true)
    await wrapper.get('#invoice-title-input').setValue('Acme Corp LLC')
    await wrapper.get('#invoice-email-input').setValue('billing@example.com')
    expect(wrapper.get('.invoice-submit').attributes('disabled')).toBeDefined()

    await wrapper.findAll('.invoice-radio input')[1].setValue(true)
    expect(wrapper.get('.invoice-submit').attributes('disabled')).toBeUndefined()
  })

  it('submits only the selected order ids and drops the tax id for an individual', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAll('.invoice-table tbody input[type="checkbox"]')[1].setValue(true)
    await fillBillingForm(wrapper)
    await wrapper.findAll('.invoice-radio input')[1].setValue(true)
    await wrapper.get('.invoice-submit').trigger('click')
    await flushPromises()

    expect(createInvoiceMock).toHaveBeenCalledWith(
      expect.objectContaining({
        entity_type: 'individual',
        order_ids: [11],
        tax_id: undefined
      })
    )
    expect(showSuccessMock).toHaveBeenCalled()
  })

  // 提交后订单已被占用,两个列表都要刷新,否则界面显示的是过期数据。
  it('refreshes both lists after a successful submit', async () => {
    const wrapper = mountView()
    await flushPromises()
    getInvoiceableOrdersMock.mockClear()
    listInvoicesMock.mockClear()

    await wrapper.findAll('.invoice-table tbody input[type="checkbox"]')[0].setValue(true)
    await fillBillingForm(wrapper)
    await wrapper.get('.invoice-submit').trigger('click')
    await flushPromises()

    expect(getInvoiceableOrdersMock).toHaveBeenCalled()
    expect(listInvoicesMock).toHaveBeenCalled()
  })

  it('deep-links into the history tab and loads the requests', async () => {
    routeState.query = { tab: 'history' }
    listInvoicesMock.mockResolvedValue({
      items: [
        {
          id: 1,
          invoice_no: 'INV-2026-A9X2B',
          amount: 45,
          status: 'issued',
          issued_invoice_no: '0440001',
          created_at: '2026-10-24T14:22:01Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('INV-2026-A9X2B')
    expect(wrapper.get('.invoice-status--issued').exists()).toBe(true)
  })

  // 发票文件地址由管理员填写,属于外部输入,不能直接当链接渲染。
  it('does not render a javascript: file url as a download link', async () => {
    routeState.query = { tab: 'history' }
    listInvoicesMock.mockResolvedValue({
      items: [
        {
          id: 1,
          invoice_no: 'INV-2026-A9X2B',
          amount: 45,
          status: 'issued',
          issued_file_url: 'javascript:alert(1)',
          created_at: '2026-10-24T14:22:01Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.html()).not.toContain('javascript:')
    expect(wrapper.find('a.invoice-action').exists()).toBe(false)
  })

  it('only offers withdraw on a pending request', async () => {
    routeState.query = { tab: 'history' }
    listInvoicesMock.mockResolvedValue({
      items: [
        { id: 1, invoice_no: 'INV-A', amount: 10, status: 'pending', created_at: '2026-10-24T00:00:00Z' },
        { id: 2, invoice_no: 'INV-B', amount: 10, status: 'rejected', created_at: '2026-10-23T00:00:00Z' }
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('button.invoice-action-danger')).toHaveLength(1)
  })

  // 撤回是破坏性操作,要先确认再执行,不能点一下就走。
  it('asks for confirmation before withdrawing a request', async () => {
    routeState.query = { tab: 'history' }
    listInvoicesMock.mockResolvedValue({
      items: [{ id: 7, invoice_no: 'INV-A', amount: 10, status: 'pending', created_at: '2026-10-24T00:00:00Z' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.dialog-stub').exists()).toBe(false)
    await wrapper.get('button.invoice-action-danger').trigger('click')

    expect(wrapper.get('.dialog-stub').exists()).toBe(true)
    expect(cancelInvoiceMock).not.toHaveBeenCalled()
  })

  it('withdraws a pending request and refreshes the invoiceable orders', async () => {
    routeState.query = { tab: 'history' }
    listInvoicesMock.mockResolvedValue({
      items: [{ id: 7, invoice_no: 'INV-A', amount: 10, status: 'pending', created_at: '2026-10-24T00:00:00Z' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    cancelInvoiceMock.mockResolvedValue(undefined)

    const wrapper = mountView()
    await flushPromises()
    getInvoiceableOrdersMock.mockClear()

    await wrapper.get('button.invoice-action-danger').trigger('click')
    await wrapper.get('.dialog-stub button.btn-danger').trigger('click')
    await flushPromises()

    expect(cancelInvoiceMock).toHaveBeenCalledWith(7)
    // 撤回释放了订单,可开票列表必须重新拉取。
    expect(getInvoiceableOrdersMock).toHaveBeenCalled()
  })

  it('surfaces a load failure as an error toast', async () => {
    getInvoiceableOrdersMock.mockRejectedValue(new Error('boom'))

    mountView()
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalled()
  })
})
