import type { CheckoutInfoResponse, PaymentOrder } from '@/types/payment'

const createOrder = (
  overrides: Partial<PaymentOrder> & Pick<PaymentOrder, 'id' | 'amount' | 'payment_type' | 'status'>
): PaymentOrder => ({
  user_id: 1,
  pay_amount: overrides.amount,
  currency: 'CNY',
  fee_rate: 0,
  out_trade_no: `preview_${overrides.id}`,
  order_type: 'balance',
  created_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
  refund_amount: 0,
  ...overrides,
})

export function createRechargePreviewData() {
  const checkout: CheckoutInfoResponse = {
    methods: {
      stripe: {
        currency: 'CNY',
        display_name: 'Stripe',
        daily_limit: 0,
        daily_used: 0,
        daily_remaining: 0,
        single_min: 10,
        single_max: 5000,
        fee_rate: 0,
        available: true,
      },
      wxpay: {
        currency: 'CNY',
        display_name: 'WeChat Pay',
        daily_limit: 0,
        daily_used: 0,
        daily_remaining: 0,
        single_min: 10,
        single_max: 5000,
        fee_rate: 0,
        available: true,
      },
      alipay: {
        currency: 'CNY',
        display_name: 'Alipay',
        daily_limit: 0,
        daily_used: 0,
        daily_remaining: 0,
        single_min: 10,
        single_max: 5000,
        fee_rate: 0,
        available: true,
      },
    },
    global_min: 10,
    global_max: 5000,
    plans: [],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 7.2,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
    stripe_publishable_key: '',
  }

  return {
    user: {
      username: 'DevUser_01',
      email: 'dev@example.com',
      balance: 1204.5,
    },
    checkout,
    recentOrders: [
      createOrder({ id: 10842, amount: 100, payment_type: 'stripe', status: 'COMPLETED' }),
      createOrder({ id: 10796, amount: 50, payment_type: 'wxpay', status: 'COMPLETED' }),
      createOrder({ id: 10731, amount: 10, payment_type: 'alipay', status: 'FAILED' }),
    ],
  }
}
