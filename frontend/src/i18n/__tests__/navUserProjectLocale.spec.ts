// The sidebar account label embeds a literal apostrophe ("{name}'s project").
// vue-i18n's message compiler treats some punctuation as syntax, so render the
// message through a real i18n instance rather than trusting the raw string.
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import en from '../locales/en'
import zh from '../locales/zh'

describe('nav.userProject', () => {
  it.each([
    ['en', en],
    ['zh', zh],
  ])('renders the possessive label with the name interpolated (%s)', (_locale, messages) => {
    const i18n = createI18n({
      legacy: false,
      locale: 'test',
      messages: { test: messages as Record<string, unknown> },
    })

    expect(i18n.global.t('nav.userProject', { name: 'alice' })).toBe("alice's project")
  })
})
