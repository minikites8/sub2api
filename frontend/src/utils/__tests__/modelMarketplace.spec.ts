import { describe, expect, it } from 'vitest'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import {
  availabilityForWindow,
  buildMarketplaceModels,
  effectiveTokenPrices,
  usdToCredits,
} from '@/utils/modelMarketplace'

describe('model marketplace aggregation', () => {
  it('groups pricing profiles by standard model', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')

    expect(models).toHaveLength(6)
    expect(gpt?.profiles).toHaveLength(2)
    expect(gpt?.rawModels).toEqual(['gpt-4.1', 'gpt-4.1-2025-04-14'])
    expect(gpt?.supportedProtocols).toEqual(['openai-chat', 'openai-responses'])
  })

  it('applies each group multiplier to displayed Credit prices', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')
    const o3 = models.find((model) => model.name === 'o3')

    expect(gpt && effectiveTokenPrices(gpt, 'input_usd_per_token')).toEqual([200, 250])
    expect(o3 && effectiveTokenPrices(o3, 'input_usd_per_token')).toEqual([200, 400])
    expect(usdToCredits(0.04)).toBe(4)
  })

  it('uses the worst monitor status and exposes each availability window', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gemini = models.find((model) => model.name === 'gemini-2.5-pro')
    const gpt = models.find((model) => model.name === 'gpt-4.1')
    const image = models.find((model) => model.name === 'gpt-image-1')

    expect(gemini?.monitoring.status).toBe('unavailable')
    expect(gemini?.monitoring.samples).toHaveLength(24)
    expect(gemini?.monitoring.samples.at(-1)?.status).toBe('unavailable')
    expect(gpt?.monitoring.samples).toHaveLength(24)
    expect(gpt?.monitoring.samples.at(-1)?.status).toBe('operational')
    expect(image?.monitoring.samples).toEqual([])
    expect(image && availabilityForWindow(image.monitoring, '7d')).toBe(99.62)
    expect(image && availabilityForWindow(image.monitoring, '30d')).toBe(99.44)
  })
})
