import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')

describe('documentation links', () => {
  it('AppHeader routes to the public documentation page', () => {
    expect(headerSource).toContain('<router-link\n          to="/docs"')
  })

  it('HomeView routes to the public documentation page', () => {
    expect(homeViewSource).toContain('<router-link to="/docs" class="stitch-nav-link">')
    expect(homeViewSource).toContain('<router-link to="/docs" class="stitch-secondary-button">')
  })

  it('KeyUsageView imports sanitizeUrl', () => {
    expect(keyUsageViewSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('KeyUsageView applies sanitizeUrl to docUrl', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl')
  })
})
