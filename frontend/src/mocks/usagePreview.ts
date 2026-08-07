import type { ApiKey, Group, ModelStat, TrendDataPoint, UsageLog, UsageStatsResponse } from '@/types'

export interface UsagePreviewData {
  stats: UsageStatsResponse
  monthToDateActualCost: number
  trend: TrendDataPoint[]
  models: ModelStat[]
  logs: UsageLog[]
  apiKeys: ApiKey[]
  groups: Group[]
}

const localDate = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const addDays = (date: Date, days: number) => {
  const result = new Date(date)
  result.setDate(result.getDate() + days)
  return result
}

const createGroup = (id: number, name: string, platform: Group['platform'], rateMultiplier: number) => ({
  id,
  name,
  description: `${name} routing group`,
  platform,
  rate_multiplier: rateMultiplier,
  status: 'active',
  subscription_type: 'standard',
}) as unknown as Group

const createApiKey = (id: number, name: string, group: Group): ApiKey => ({
  id,
  key: `sk-preview-${id}-usage-dashboard`,
  name,
  user_id: 1,
  group_id: group.id,
  group,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: new Date().toISOString(),
  last_used_ip: '203.0.113.24',
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: addDays(new Date(), -90).toISOString(),
  updated_at: new Date().toISOString(),
  current_concurrency: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const createUsageLog = (
  id: number,
  createdAt: Date,
  model: string,
  apiKey: ApiKey,
  duration: number,
  inputTokens: number,
  outputTokens: number,
  actualCost: number,
): UsageLog => ({
  id,
  user_id: 1,
  api_key_id: apiKey.id,
  account_id: id % 5,
  request_id: `req_preview_${String(id).padStart(5, '0')}`,
  node_id: `edge-cn-${String((id % 3) + 1).padStart(2, '0')}`,
  model,
  service_tier: id % 4 === 0 ? 'priority' : 'standard',
  reasoning_effort: id % 3 === 0 ? 'medium' : null,
  inbound_endpoint: model.startsWith('claude')
    ? '/v1/messages'
    : model.startsWith('gemini')
      ? '/v1beta/models/generateContent'
      : model.startsWith('gpt')
        ? '/v1/responses'
        : '/v1/chat/completions',
  upstream_endpoint: null,
  group_id: apiKey.group_id,
  subscription_id: null,
  input_tokens: inputTokens,
  output_tokens: outputTokens,
  cache_creation_tokens: Math.round(inputTokens * 0.04),
  cache_read_tokens: Math.round(inputTokens * 0.22),
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: actualCost * 0.52,
  output_cost: actualCost * 0.3,
  cache_creation_cost: actualCost * 0.05,
  cache_read_cost: actualCost * 0.13,
  total_cost: actualCost * 1.35,
  actual_cost: actualCost,
  rate_multiplier: Number(apiKey.group?.rate_multiplier || 1),
  long_context_billing_applied: false,
  billing_type: 0,
  request_type: id % 3 === 0 ? 'stream' : 'sync',
  stream: id % 3 === 0,
  duration_ms: duration,
  first_token_ms: Math.round(duration * 0.16),
  video_count: 0,
  video_resolution: null,
  video_duration_seconds: null,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  media_type: null,
  user_agent: id % 3 === 0
    ? 'claude-cli/2.1.14 (external, cli)'
    : id % 3 === 1
      ? 'openai-node/4.71.1'
      : 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)',
  ip_address: `203.0.113.${20 + (id % 18)}`,
  cache_ttl_overridden: false,
  billing_mode: 'token',
  created_at: createdAt.toISOString(),
  api_key: apiKey,
  group: apiKey.group,
})

export function createUsagePreviewData(): UsagePreviewData {
  const now = new Date()
  const groups = [
    createGroup(1, '统一网关', 'openai', 1),
    createGroup(2, 'Claude 高性能', 'anthropic', 1.2),
    createGroup(3, 'Gemini 路由', 'gemini', 0.85),
  ]
  const apiKeys = [
    createApiKey(101, '生产环境网关', groups[0]),
    createApiKey(102, '研发自动化', groups[1]),
    createApiKey(103, '内容工作流', groups[2]),
  ]
  const modelNames = ['gpt-4.1', 'claude-sonnet-4', 'gemini-2.5-pro', 'grok-4']
  const logs: UsageLog[] = []

  for (let dayIndex = 0; dayIndex < 7; dayIndex++) {
    const date = addDays(now, dayIndex - 6)
    for (let hourIndex = 0; hourIndex < 12; hourIndex++) {
      for (let sample = 0; sample < 2; sample++) {
        const id = dayIndex * 24 + hourIndex * 2 + sample + 1
        const createdAt = new Date(date)
        createdAt.setHours(hourIndex * 2, 9 + sample * 28, 12 + id % 41, 0)
        const model = modelNames[(id + dayIndex) % modelNames.length]
        const durationBase = 72 + ((id * 73) % 520)
        const duration = id % 23 === 0 ? 1680 + id * 3 : durationBase
        const inputTokens = 880 + ((id * 347) % 7600)
        const outputTokens = 190 + ((id * 173) % 2100)
        const actualCost = 0.018 + ((id * 19) % 120) / 1000
        logs.push(createUsageLog(
          id,
          createdAt,
          model,
          apiKeys[id % apiKeys.length],
          duration,
          inputTokens,
          outputTokens,
          actualCost,
        ))
      }
    }
  }

  logs.sort((a, b) => b.created_at.localeCompare(a.created_at))

  const trend: TrendDataPoint[] = Array.from({ length: 7 }, (_, index) => {
    const date = addDays(now, index - 6)
    const input = 388_000 + index * 49_000 + (index % 2) * 74_000
    const output = 138_000 + index * 27_000
    const cacheCreation = 18_000 + index * 4_600
    const cacheRead = 112_000 + index * 21_000
    return {
      date: localDate(date),
      requests: 2_480 + index * 310,
      input_tokens: input,
      output_tokens: output,
      cache_creation_tokens: cacheCreation,
      cache_read_tokens: cacheRead,
      total_tokens: input + output + cacheCreation + cacheRead,
      cost: 12.7 + index * 1.94,
      actual_cost: 8.9 + index * 1.37,
    }
  })

  const models: ModelStat[] = [
    { model: 'gpt-4.1', requests: 7_840, input_tokens: 1_420_000, output_tokens: 520_000, cache_creation_tokens: 98_000, cache_read_tokens: 610_000, total_tokens: 2_648_000, cost: 49.72, actual_cost: 34.81 },
    { model: 'claude-sonnet-4', requests: 5_930, input_tokens: 1_080_000, output_tokens: 440_000, cache_creation_tokens: 126_000, cache_read_tokens: 490_000, total_tokens: 2_136_000, cost: 38.14, actual_cost: 27.13 },
    { model: 'gemini-2.5-pro', requests: 4_260, input_tokens: 810_000, output_tokens: 310_000, cache_creation_tokens: 42_000, cache_read_tokens: 360_000, total_tokens: 1_522_000, cost: 22.35, actual_cost: 16.92 },
    { model: 'grok-4', requests: 3_140, input_tokens: 620_000, output_tokens: 248_000, cache_creation_tokens: 31_000, cache_read_tokens: 214_000, total_tokens: 1_113_000, cost: 15.26, actual_cost: 10.35 },
  ]

  return {
    monthToDateActualCost: 108.64,
    stats: {
      period: '7d',
      total_requests: models.reduce((sum, item) => sum + item.requests, 0),
      total_input_tokens: models.reduce((sum, item) => sum + item.input_tokens, 0),
      total_output_tokens: models.reduce((sum, item) => sum + item.output_tokens, 0),
      total_cache_tokens: models.reduce((sum, item) => sum + item.cache_creation_tokens + item.cache_read_tokens, 0),
      total_cache_read_tokens: models.reduce((sum, item) => sum + item.cache_read_tokens, 0),
      total_cache_creation_tokens: models.reduce((sum, item) => sum + item.cache_creation_tokens, 0),
      total_tokens: models.reduce((sum, item) => sum + item.total_tokens, 0),
      total_cost: models.reduce((sum, item) => sum + item.cost, 0),
      total_actual_cost: models.reduce((sum, item) => sum + item.actual_cost, 0),
      average_duration_ms: 245,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    },
    trend,
    models,
    logs,
    apiKeys,
    groups,
  }
}
