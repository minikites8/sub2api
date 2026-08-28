import type { PublicTransitSnapshot } from '@/api/publicTransit'

const perToken = (input: number, output: number, cacheWrite?: number, cacheRead?: number) => ({
  input_usd_per_token: input / 1_000_000,
  output_usd_per_token: output / 1_000_000,
  cache_write_usd_per_token: cacheWrite == null ? undefined : cacheWrite / 1_000_000,
  cache_read_usd_per_token: cacheRead == null ? undefined : cacheRead / 1_000_000,
})

export function createModelMarketplacePreviewSnapshot(): PublicTransitSnapshot {
  const now = new Date().toISOString()
  const buckets = (baseLatency: number, incidents: Record<number, string> = {}) => Array.from({ length: 24 }, (_, index) => ({
    bucket_start: new Date(new Date(now).getTime() - (23 - index) * 5 * 60_000).toISOString(),
    status: incidents[index] || 'operational',
    success_rate: incidents[index] ? 94 : 100,
    duration_p50_ms: baseLatency + ((index * 37) % 190),
  }))
  return {
    schema_version: '2.1.0',
    system: 'Sub2API Preview',
    generated_at: now,
    station: {
      name: 'Sub2API',
      homepage_url: '/home',
      price_url: '/monitor',
      monitor_url: '/monitor',
      system_type: 'AI Gateway',
    },
    billing: {
      currency: 'USD',
      credit_currency: 'USD',
      recharge_ratio: 'Prices shown in USD ($)',
      recharge_multiplier: 1,
      recharge_multiplier_unit: 'USD/USD',
      minimum_top_up: 10,
      model_basis_price: 'USD',
      model_price_unit: 'USD',
      standardized_price_version: '2026-07',
    },
    groups: [
      {
        name: '标准路由',
        platform: 'openai',
        subscription_type: 'standard',
        rate_multiplier: 1,
        is_exclusive: false,
        cache_usage: { last_24h: { period: '24h', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 }, last_7d: { period: '7d', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 } },
        models: [
          { standard_model: 'gpt-4.1', raw_model: 'gpt-4.1-2025-04-14', platform: 'openai', billing_mode: 'token', price_source: 'standard', price: perToken(2, 8, 2.5, 0.5), source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['openai-responses', 'openai-chat'] },
          { standard_model: 'o3', raw_model: 'o3-2025-04-16', platform: 'openai', billing_mode: 'token', price_source: 'standard', price: perToken(2, 8, 2, 0.5), source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['openai-responses'], intervals: [{ min_tokens: 0, max_tokens: 200_000, tier_label: 'Standard', ...perToken(2, 8) }, { min_tokens: 200_001, tier_label: 'Long context', ...perToken(4, 16) }] },
          { standard_model: 'gpt-image-1', raw_model: 'gpt-image-1', platform: 'openai', billing_mode: 'per_request', price_source: 'standard', price: { per_request_usd: 0.04, image_size_prices: { '1k': 0.04, '2k': 0.07, '4k': 0.12 } }, source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['openai-images'] },
        ],
      },
      {
        name: '高速路由',
        platform: 'openai',
        subscription_type: 'priority',
        rate_multiplier: 1.25,
        is_exclusive: false,
        cache_usage: { last_24h: { period: '24h', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 }, last_7d: { period: '7d', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 } },
        models: [
          { standard_model: 'gpt-4.1', raw_model: 'gpt-4.1', platform: 'openai', billing_mode: 'token', price_source: 'custom', price: perToken(2, 8, 2.5, 0.5), source: { upstream_type: 'official', account_pool_type: 'dedicated', disclosure: '独立高优先级路由' }, supported_protocols: ['openai-responses', 'openai-chat'] },
        ],
      },
      {
        name: 'Claude 专线',
        platform: 'anthropic',
        subscription_type: 'standard',
        rate_multiplier: 0.9,
        is_exclusive: false,
        cache_usage: { last_24h: { period: '24h', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 }, last_7d: { period: '7d', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 } },
        models: [
          { standard_model: 'claude-sonnet-4', raw_model: 'claude-sonnet-4-20250514', platform: 'anthropic', billing_mode: 'token', price_source: 'standard', price: perToken(3, 15, 3.75, 0.3), source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['anthropic-messages', 'openai-chat'] },
          { standard_model: 'claude-opus-4', raw_model: 'claude-opus-4-20250514', platform: 'anthropic', billing_mode: 'token', price_source: 'standard', price: perToken(15, 75, 18.75, 1.5), source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['anthropic-messages'] },
        ],
      },
      {
        name: 'Gemini 路由',
        platform: 'gemini',
        subscription_type: 'standard',
        rate_multiplier: 0.75,
        is_exclusive: false,
        cache_usage: { last_24h: { period: '24h', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 }, last_7d: { period: '7d', input_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, cache_hit_rate: 0 } },
        models: [
          { standard_model: 'gemini-2.5-pro', raw_model: 'gemini-2.5-pro-preview-06-05', platform: 'gemini', billing_mode: 'token', price_source: 'standard', price: perToken(1.25, 10, 1.25, 0.31), source: { upstream_type: 'official', account_pool_type: 'pooled', disclosure: '官方账户池' }, supported_protocols: ['gemini-generate-content', 'openai-chat'] },
        ],
      },
    ],
    monitoring: [
      { platform: 'openai', model: 'gpt-4.1', status: 'operational', availability_7d: 99.98, availability_15d: 99.94, availability_30d: 99.91, ttft_p50_7d_ms: 220, duration_p50_7d_ms: 842, latest_duration_p50_ms: 714, data_through: now, coverage_complete: true, buckets: buckets(714) },
      { platform: 'openai', model: 'o3', status: 'operational', availability_7d: 99.87, availability_15d: 99.81, availability_30d: 99.76, ttft_p50_7d_ms: 410, duration_p50_7d_ms: 1430, latest_duration_p50_ms: 1280, data_through: now, coverage_complete: true, buckets: buckets(1280) },
      { platform: 'openai', model: 'gpt-image-1', status: 'operational', availability_7d: 99.62, availability_15d: 99.58, availability_30d: 99.44, ttft_p50_7d_ms: 0, duration_p50_7d_ms: 6950, latest_duration_p50_ms: 6720, data_through: now, coverage_complete: true, buckets: buckets(6720) },
      { platform: 'anthropic', model: 'claude-sonnet-4', status: 'degraded', availability_7d: 98.72, availability_15d: 99.14, availability_30d: 99.38, ttft_p50_7d_ms: 360, duration_p50_7d_ms: 1090, latest_duration_p50_ms: 1535, data_through: now, coverage_complete: true, buckets: buckets(1535, { 18: 'degraded', 23: 'degraded' }) },
      { platform: 'anthropic', model: 'claude-opus-4', status: 'operational', availability_7d: 99.64, availability_15d: 99.55, availability_30d: 99.48, ttft_p50_7d_ms: 690, duration_p50_7d_ms: 1760, latest_duration_p50_ms: 1890, data_through: now, coverage_complete: true, buckets: buckets(1890) },
      { platform: 'gemini', model: 'gemini-2.5-pro', status: 'unavailable', availability_7d: 96.44, availability_15d: 97.85, availability_30d: 98.61, ttft_p50_7d_ms: 520, duration_p50_7d_ms: 1320, latest_duration_p50_ms: 0, data_through: now, coverage_complete: true, buckets: buckets(1250, { 20: 'unavailable', 23: 'unavailable' }) },
    ],
    cache: { supported: true, write_unit: 'token', read_unit: 'token', hit_rate: 78.4, hit_rate_period: '7d' },
    disclosure: { upstream_type: 'mixed', account_pool_type: 'pooled', is_mixed: true, is_reverse: false, note: 'Preview data' },
    limits: { concurrency: 'dynamic', rpm: 'dynamic', tpm: 'dynamic', over_limit_behavior: 'queue' },
    completeness: { has_recharge_ratio: true, has_group_multipliers: true, has_model_pricing: true, has_monitoring: true, has_source_disclosure: true, warnings: [] },
    endpoints: { discovery_url: '/.well-known/ai-transit.json', snapshot_url: '/api/public/transit/v1/snapshot', public_page_url: '/monitor' },
  }
}
