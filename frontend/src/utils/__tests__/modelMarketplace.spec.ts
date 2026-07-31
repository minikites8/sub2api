import { describe, expect, it } from 'vitest'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import {
  availabilityForWindow,
  buildMarketplaceModels,
  effectiveTokenPrices,
  effectiveVideoPrices,
  hasTokenPricing,
  modelDeveloper,
  usdToCredits,
} from '@/utils/modelMarketplace'

describe('model marketplace aggregation', () => {
  it('groups pricing profiles by standard model', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')

    expect(models).toHaveLength(8)
    expect(gpt?.profiles).toHaveLength(2)
    expect(gpt?.rawModels).toEqual(['gpt-4.1', 'gpt-4.1-2025-04-14'])
    expect(gpt?.supportedProtocols).toEqual(['openai-chat', 'openai-responses'])
    expect(gpt?.developer).toBe('OpenAI')
  })

  it('resolves model developers independently from channel providers', () => {
    expect(modelDeveloper('doubao-seedance-2.0')).toBe('ByteDance')
    expect(modelDeveloper('claude-sonnet-4')).toBe('Anthropic')
    expect(modelDeveloper('gemini-2.5-pro')).toBe('Google')
    expect(modelDeveloper('custom-routed-model')).toBe('')
  })

  it('aggregates only providers enabled by channel configuration', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const standard = snapshot.groups.find((group) => group.name === '标准路由')!
    const priority = snapshot.groups.find((group) => group.name === '高速路由')!
    standard.provider_visible = true
    priority.provider_visible = false

    const models = buildMarketplaceModels(snapshot)
    const gpt = models.find((model) => model.name === 'gpt-4.1')!
    const claude = models.find((model) => model.name === 'claude-sonnet-4')!

    expect(gpt.platforms).toEqual(['openai'])
    expect(gpt.visiblePlatforms).toEqual(['openai'])
    expect(gpt.profiles.map((profile) => profile.providerVisible)).toEqual([true, false])
    expect(claude.visiblePlatforms).toEqual([])
    expect(claude.profiles[0].providerVisible).toBe(false)
  })

  it('collapses mapped aliases from one pricing entry into one marketplace item', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const videoGroup = snapshot.groups.find((group) => group.name === '百度 VOD 视频')!
    const versionedModel = videoGroup.models.find((model) => model.standard_model === 'doubao-seedance-2-0-260128')!
    const aliases = ['doubao-seedance-2.0', 'doubao-seedance-2.0-260128']
    Object.assign(versionedModel, {
      standard_model: aliases[0],
      raw_model: aliases[1],
      pricing_models: aliases,
    })
    videoGroup.models.push({
      ...versionedModel,
      standard_model: aliases[1],
      raw_model: aliases[1],
      pricing_models: aliases,
    })
    const tokenGroup = snapshot.groups.find((group) => group.name === 'Seedance Token 路由')!
    Object.assign(tokenGroup.models[0], {
      standard_model: aliases[1],
      raw_model: aliases[1],
      pricing_models: aliases,
    })

    const models = buildMarketplaceModels(snapshot)
    const seedanceModels = models.filter((model) => model.name.startsWith('doubao-seedance-2.0'))

    expect(seedanceModels).toHaveLength(1)
    expect(seedanceModels[0].name).toBe(aliases[0])
    expect(seedanceModels[0].rawModels).toEqual(aliases)
    expect(seedanceModels[0].profiles).toHaveLength(2)
    expect(seedanceModels[0].billingModes).toEqual(['token', 'video'])
  })

  it('collapses capability variants sharing a pricing entry without model mappings', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const videoGroup = snapshot.groups.find((group) => group.name === '百度 VOD 视频')!
    const model = videoGroup.models.find((item) => item.standard_model === 'happyhorse-1.1-t2v')!
    const aliases = [
      'happyhorse-1.0-t2v',
      'happyhorse-1.0-i2v',
      'happyhorse-1.0-r2v',
      'happyhorse-1.0-video-edit',
    ]
    Object.assign(model, {
      standard_model: aliases[0],
      raw_model: aliases[0],
      pricing_models: aliases,
    })
    for (const alias of aliases.slice(1)) {
      videoGroup.models.push({
        ...model,
        standard_model: alias,
        raw_model: alias,
        pricing_models: aliases,
      })
    }
    videoGroup.models.sort((left, right) => left.standard_model.localeCompare(right.standard_model))

    const models = buildMarketplaceModels(snapshot)
    const happyHorseModels = models.filter((item) => item.rawModels.some((name) => aliases.includes(name)))

    expect(happyHorseModels).toHaveLength(1)
    expect(happyHorseModels[0].name).toBe(aliases[0])
    expect(happyHorseModels[0].rawModels).toEqual([...aliases].sort())
    expect(happyHorseModels[0].profiles).toHaveLength(1)
    expect(happyHorseModels[0].profiles[0].model.standard_model).toBe(aliases[0])
    expect(happyHorseModels[0].billingModes).toEqual(['video'])
  })

  it('collapses HappyHorse 1.1 capability variants from one pricing entry', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const videoGroup = snapshot.groups.find((group) => group.name === '百度 VOD 视频')!
    const model = videoGroup.models.find((item) => item.standard_model === 'happyhorse-1.1-t2v')!
    const aliases = [
      'happyhorse-1.1-t2v',
      'happyhorse-1.1-i2v',
      'happyhorse-1.1-r2v',
      'happyhorse-1.1-video-edit',
    ]
    Object.assign(model, { pricing_models: aliases })
    for (const alias of aliases.slice(1)) {
      videoGroup.models.push({
        ...model,
        standard_model: alias,
        raw_model: alias,
        pricing_models: aliases,
      })
    }

    const models = buildMarketplaceModels(snapshot)
    const happyHorseModels = models.filter((item) => item.rawModels.some((name) => aliases.includes(name)))

    expect(happyHorseModels).toHaveLength(1)
    expect(happyHorseModels[0].name).toBe(aliases[0])
    expect(happyHorseModels[0].rawModels).toEqual([...aliases].sort())
    expect(happyHorseModels[0].profiles).toHaveLength(1)
  })

  it('applies the video multiplier to Credits per second prices', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const seedance = models.find((model) => model.name === 'doubao-seedance-2-0-260128')
    const happyHorse = models.find((model) => model.name === 'happyhorse-1.1-t2v')

    expect(seedance?.billingModes).toEqual(['token', 'video'])
    expect(seedance && effectiveVideoPrices(seedance)).toEqual([
      { resolution: '480p', values: [23] },
      { resolution: '720p', values: [49.5] },
      { resolution: '1080p', values: [124] },
      { resolution: '4k', values: [252.5] },
    ])
    expect(seedance && effectiveTokenPrices(seedance, 'output_usd_per_token')).toEqual([50])
    expect(seedance && hasTokenPricing(seedance)).toBe(true)
    expect(happyHorse && effectiveVideoPrices(happyHorse)).toEqual([
      { resolution: '720p', values: [45] },
      { resolution: '1080p', values: [60] },
    ])
  })

  it('applies each group multiplier to displayed Credit prices', () => {
    const models = buildMarketplaceModels(createModelMarketplacePreviewSnapshot())
    const gpt = models.find((model) => model.name === 'gpt-4.1')
    const o3 = models.find((model) => model.name === 'o3')

    expect(gpt && effectiveTokenPrices(gpt, 'input_usd_per_token')).toEqual([200, 250])
    expect(o3 && effectiveTokenPrices(o3, 'input_usd_per_token')).toEqual([200, 400])
    expect(usdToCredits(0.04)).toBe(4)
  })

  it('applies the video multiplier to conditional video token prices', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const tokenGroup = snapshot.groups.find((group) => group.name === 'Seedance Token 路由')!
    tokenGroup.video_rate_multiplier = 0.5
    const seedance = tokenGroup.models[0]
    seedance.billing_mode = 'video_token'
    seedance.price = {}
    seedance.intervals = [
      { min_tokens: 0, tier_label: '720p:text', output_usd_per_token: 46e-6 },
      { min_tokens: 0, tier_label: '720p:video', output_usd_per_token: 28e-6 },
    ]

    const model = buildMarketplaceModels(snapshot).find((item) => item.name === 'doubao-seedance-2-0-260128')!
    expect(model.billingModes).toEqual(['video', 'video_token'])
    expect(effectiveTokenPrices(model, 'output_usd_per_token')).toEqual([1400, 2300])
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
