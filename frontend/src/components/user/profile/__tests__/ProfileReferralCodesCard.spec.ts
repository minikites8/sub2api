import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileReferralCodesCard from '@/components/user/profile/ProfileReferralCodesCard.vue'
import type { User } from '@/types'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.referralCodesTitle') return 'Promo & Affiliate Codes'
        if (key === 'profile.referralCodesDescription') return 'Review account promo and affiliate info'
        if (key === 'profile.myAffiliateCode') return 'My Affiliate Code'
        if (key === 'profile.affiliateInviterBound') return 'Affiliate inviter bound'
        if (key === 'profile.affiliateInviterEmpty') return 'No affiliate inviter bound'
        if (key === 'profile.usedAffiliateCode') return 'Used Affiliate Code'
        if (key === 'profile.usedPromoCodes') return 'Used Promo Codes'
        if (key === 'profile.noUsedPromoCodes') return 'No platform promo codes used yet'
        if (key === 'profile.promoBonusAmount') return `Bonus ${params?.amount || ''}`.trim()
        if (key === 'profile.promoUsed') return 'Used'
        if (key === 'common.none') return 'None'
        return key
      }
    })
  }
})

function createUser(overrides: Partial<User> = {}): User {
  return {
    id: 5,
    username: 'alice',
    email: 'alice@example.com',
    avatar_url: null,
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides
  }
}

describe('ProfileReferralCodesCard', () => {
  it('renders empty states when there is no affiliate code or promo usage', () => {
    const wrapper = mount(ProfileReferralCodesCard, {
      props: { user: createUser() }
    })

    expect(wrapper.text()).toContain('None')
    expect(wrapper.text()).toContain('No affiliate inviter bound')
    expect(wrapper.text()).toContain('No platform promo codes used yet')
  })

  it('renders used promo codes and affiliate code information', () => {
    const wrapper = mount(ProfileReferralCodesCard, {
      props: {
        user: createUser({
          affiliate: {
            aff_code: 'AFF123',
            inviter_id: 99,
            inviter_aff_code: 'INVITER99'
          },
          used_promo_codes: [
            {
              code: 'PARTNER50',
              bonus_amount: 5,
              used_at: '2026-06-17T08:00:00Z'
            }
          ]
        })
      }
    })

    expect(wrapper.text()).toContain('AFF123')
    expect(wrapper.text()).toContain('Affiliate inviter bound')
    expect(wrapper.text()).toContain('Used Affiliate Code')
    expect(wrapper.text()).toContain('INVITER99')
    expect(wrapper.text()).toContain('PARTNER50')
    expect(wrapper.text()).toContain('Bonus $5.00')
  })
})
