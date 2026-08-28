import { describe, expect, it } from 'vitest'
import { createModelMarketplacePreviewSnapshot } from '@/mocks/modelMarketplacePreview'
import {
  availabilityForWindow,
  applyGroupMultiplier,
  buildMarketplaceModels,
  effectiveTokenPrices,
  hasTokenPricing,
  modelDeveloper,
  monitoringForWindow,
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

  it('keeps passive monitoring isolated per enabled pricing group and window', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const group = snapshot.groups[0]
    const now = snapshot.generated_at
    group.monitoring_enabled = true
    group.monitoring = [{
      platform: 'openai',
      model: 'gpt-4.1',
      status: 'operational',
      availability_7d: 99,
      availability_15d: 99,
      availability_30d: 99,
      coverage_complete: true,
      buckets: [],
      windows: {
        '90m': {
          status: 'degraded',
          availability: 97.5,
          coverage_complete: true,
          buckets: [{ bucket_start: now, status: 'degraded', success_rate: 97.5 }],
        },
      },
    }]

    const gpt = buildMarketplaceModels(snapshot).find((model) => model.name === 'gpt-4.1')!
    const monitoredProfile = gpt.profiles.find((profile) => profile.groupName === group.name)!
    const otherProfile = gpt.profiles.find((profile) => profile.groupName !== group.name)!

    expect(monitoredProfile.monitoringEnabled).toBe(true)
    expect(availabilityForWindow(monitoredProfile.monitoring!, '90m')).toBe(97.5)
    expect(otherProfile.monitoringEnabled).toBe(false)
  })

  it('treats a window with no requests as unknown instead of zero availability', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.windows = {
      '90m': {
        status: 'unmonitored',
        availability: 0,
        coverage_complete: true,
        buckets: [],
      },
    }

    const model = buildMarketplaceModels(snapshot).find((item) => item.name === 'gpt-4.1')!
    const window = monitoringForWindow(model.monitoring, '90m')

    expect(window.status).toBe('unmonitored')
    expect(window.availability).toBeUndefined()
    expect(availabilityForWindow(model.monitoring, '90m')).toBeUndefined()
  })

  it('keeps passive timeline success rates for V2-style color scoring', () => {
    const model = buildMarketplaceModels(createModelMarketplacePreviewSnapshot()).find((item) => item.name === 'gpt-4.1')!

    expect(model.monitoring.samples[0].successRate).toBe(100)
  })
})
