// 这个页面第一次上线时是纯黑屏：文案里的 'billing@example.com' 未转义 @，
// vue-i18n 在渲染时抛 "Invalid linked format"，整棵组件树炸掉。
// InvoiceView.spec.ts 把 t 换成了 mock，看不到这个问题。
// 这里用真实 i18n 消息挂载，确保页面能真正渲染出来。
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import InvoiceView from '@/views/user/InvoiceView.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

vi.mock('@/api/invoice', () => ({
  default: {
    getInvoiceableOrders: vi.fn().mockResolvedValue([]),
    listInvoices: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 }),
    createInvoice: vi.fn(),
    cancelInvoice: vi.fn()
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { email: 'dev@example.com' } })
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ path: '/invoices', query: {} }),
    useRouter: () => ({ replace: vi.fn() })
  }
})

describe('InvoiceView with real i18n messages', () => {
  it.each([
    ['zh', zh],
    ['en', en],
  ])('renders its copy without a message-compiler error (%s)', async (locale, messages) => {
    const i18n = createI18n({
      legacy: false,
      locale,
      fallbackLocale: locale,
      messages: { [locale]: messages as Record<string, unknown> },
    })

    const wrapper = mount(InvoiceView, {
      global: {
        plugins: [i18n],
        stubs: { AppLayout: { template: '<div><slot /></div>' } },
      },
    })
    await flushPromises()

    // 渲染失败时这些节点根本不存在，页面就是一片空白。
    expect(wrapper.get('.invoice-title').text().length).toBeGreaterThan(0)
    expect(wrapper.findAll('.invoice-tabs button')).toHaveLength(2)

    // 未解析的 key 会原样渲染成 "invoice.xxx"，说明文案没接上。
    expect(wrapper.text()).not.toContain('invoice.apply.')
    expect(wrapper.text()).not.toContain('invoice.tabs.')

    // 邮箱占位符里的 @ 必须以字面量渲染，而不是被当成链接消息语法。
    const emailInput = wrapper.get('#invoice-email-input').attributes('placeholder')
    expect(emailInput).toBe('billing@example.com')
  })
})
