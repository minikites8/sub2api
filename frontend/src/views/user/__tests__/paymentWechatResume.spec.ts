import { describe, expect, it } from 'vitest'
import { parseWechatResumeRoute, stripWechatResumeQuery } from '../paymentWechatResume'

describe('parseWechatResumeRoute', () => {
  it('restores the quantity and combined amount for legacy subscription callbacks', () => {
    const result = parseWechatResumeRoute({
      wechat_resume: '1', openid: 'openid', order_type: 'subscription', plan_id: '7', quantity: '5',
    }, [{ id: 7, price: 9.9 }] as Parameters<typeof parseWechatResumeRoute>[1], 0)
    expect(result).toMatchObject({ quantity: 5, orderAmount: 49.5, planId: 7 })
  })

  it('prefers the opaque resume token over legacy openid query params', () => {
    expect(parseWechatResumeRoute({
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'subscription',
      plan_id: '7',
      quantity: '3',
      promo_code: 'SUB-80',
    }, [], 88)).toEqual({
      wechatResumeToken: 'resume-token-123',
      paymentType: 'wxpay',
      orderType: 'subscription',
      orderAmount: 0,
      planId: 7,
      quantity: 3,
      promoCode: 'SUB-80',
    })
  })

  it('falls back to legacy openid-based resume when opaque token is absent', () => {
    expect(parseWechatResumeRoute({
      wechat_resume: '1',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'balance',
    }, [], 88)).toEqual({
      openid: 'openid-123',
      paymentType: 'wxpay',
      orderType: 'balance',
      orderAmount: 12.5,
      planId: undefined,
    })
  })
})

describe('stripWechatResumeQuery', () => {
  it('removes both opaque-token and legacy resume params from the route query', () => {
    expect(stripWechatResumeQuery({
      foo: 'bar',
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'subscription',
      plan_id: '7',
      quantity: '5',
      state: 'state-123',
      scope: 'snsapi_base',
      promo_code: 'SUB-80',
    })).toEqual({
      foo: 'bar',
    })
  })
})
