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
        status: 'operational',
        availability: 0,
        coverage_complete: true,
        metrics: {
          has_requests: false,
          success_rate: 0,
          error_rate: 0,
          cache_rate: 0,
          ttft: {},
          duration: {},
        },
        health: {
          overall: 'unknown',
          error_rate: 'unknown',
          ttft: 'unknown',
          cache: 'unknown',
          minimum_sample: 50,
        },
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

  it('excludes no-request aliases from V2 availability aggregation', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const group = snapshot.groups[0]
    group.monitoring_enabled = true
    group.monitoring = [
      {
        platform: 'openai',
        model: 'gpt-4.1',
        status: 'unmonitored',
        availability_7d: 0,
        availability_15d: 0,
        availability_30d: 0,
        coverage_complete: true,
        buckets: [],
        windows: {
          '90m': {
            status: 'operational',
            availability: 0,
            coverage_complete: true,
            metrics: {
              has_requests: false,
              success_rate: 0,
              error_rate: 0,
              cache_rate: 0,
              ttft: {},
              duration: {},
            },
            health: {
              overall: 'unknown',
              error_rate: 'unknown',
              ttft: 'unknown',
              cache: 'unknown',
              minimum_sample: 50,
            },
            buckets: [],
          },
        },
      },
      {
        platform: 'openai',
        model: 'gpt-4.1-2025-04-14',
        status: 'operational',
        availability_7d: 98,
        availability_15d: 97,
        availability_30d: 96,
        coverage_complete: true,
        buckets: [],
        windows: {
          '90m': {
            status: 'operational',
            availability: 95,
            coverage_complete: true,
            buckets: [{
              bucket_start: snapshot.generated_at,
              status: 'operational',
              success_rate: 95,
            }],
          },
        },
      },
    ]

    const model = buildMarketplaceModels(snapshot).find((item) => item.name === 'gpt-4.1')!
    const monitoring = model.profiles.find((profile) => profile.groupName === group.name)!.monitoring!

    expect(monitoring.status).toBe('operational')
    expect(monitoring.availability7d).toBe(98)
    expect(monitoringForWindow(monitoring, '90m').availability).toBe(95)
    expect(monitoringForWindow(monitoring, '90m').samples[0].successRate).toBe(95)
  })

  it('uses V2 metrics and health when legacy status fields disagree', () => {
    const snapshot = createModelMarketplacePreviewSnapshot()
    const monitor = snapshot.monitoring.find((item) => item.model === 'gpt-4.1')!
    monitor.windows = {
      '90m': {
        status: 'operational',
        availability: 0,
        latest_duration_p50_ms: 123,
        coverage_complete: true,
        metrics: {
          has_requests: true,
          success_rate: 0.95,
          error_rate: 0.05,
          cache_rate: 0,
          ttft: { p50_ms: 200 },
          duration: { p50_ms: 800, avg_ms: 900 },
        },
        health: {
          overall: 'healthy',
          error_rate: 'healthy',
          ttft: 'healthy',
          cache: 'unknown',
          score: 90,
          success_rate_score: 95,
          minimum_sample: 50,
        },
        buckets: [],
      },
    }

    const model = buildMarketplaceModels(snapshot).find((item) => item.name === 'gpt-4.1')!
    const window = monitoringForWindow(model.monitoring, '90m')

    expect(window.status).toBe('operational')
    expect(window.hasRequests).toBe(true)
    expect(window.availability).toBe(95)
    expect(window.healthScore).toBe(90)
    expect(window.latestLatencyMs).toBe(800)
    expect(window.avgLatencyMs).toBe(900)
  })
})
