import { describe, expect, it } from 'vitest'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import {
  availabilityForWindow,
  applyGroupMultiplier,
  buildMarketplaceModels,
  effectiveTokenPrices,
  hasTokenPricing,
  modelDeveloper,
} from '@/utils/modelMarketplace'

describe('model marketplace aggregation', () => {
  it('groups pricing profiles by standard model and resolves developer', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')

    expect(models).toHaveLength(6)
    expect(gpt?.profiles).toHaveLength(2)
    expect(gpt?.rawModels).toEqual(['gpt-4.1', 'gpt-4.1-2025-04-14'])
    expect(gpt?.developer).toBe('OpenAI')
  })

  it('resolves model developers independently from channel providers', () => {
    expect(modelDeveloper('claude-sonnet-4')).toBe('Anthropic')
    expect(modelDeveloper('gemini-2.5-pro')).toBe('Google')
    expect(modelDeveloper('custom-routed-model')).toBe('')
  })

  it('aggregates provider visibility and aliases', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const standard = snapshot.groups.find((group) => group.name === '标准路由')!
    const priority = snapshot.groups.find((group) => group.name === '高速路由')!
    standard.provider_visible = true
    priority.provider_visible = false
    const model = standard.models[0]
    Object.assign(model, { pricing_models: ['gpt-4.1', 'gpt-4.1-latest'] })

    const models = buildMarketplaceModels(snapshot)
    const gpt = models.find((item) => item.name === 'gpt-4.1')!

    expect(gpt.visiblePlatforms).toEqual(['openai'])
    expect(gpt.profiles.map((profile) => profile.providerVisible)).toEqual([true, false])
    expect(gpt.rawModels).toContain('gpt-4.1-latest')
  })

  it('applies group multipliers to USD prices and aggregates V2 monitoring', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')!
    const gemini = models.find((model) => model.name === 'gemini-2.5-pro')!

    expect(effectiveTokenPrices(gpt, 'input_usd_per_token')).toEqual([2, 2.5])
    expect(applyGroupMultiplier(0.04)).toBe(0.04)
    expect(gemini.monitoring.status).toBe('unavailable')
    expect(availabilityForWindow(gemini.monitoring, '30d')).toBe(98.61)
    expect(hasTokenPricing(gpt)).toBe(true)
  })
})
