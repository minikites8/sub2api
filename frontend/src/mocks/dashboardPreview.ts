import type { UserDashboardStats } from '@/api/usage'
import type { ApiKey, DailyCheckinStatus, ModelStat, TrendDataPoint } from '@/types'

export interface DashboardPreviewData {
  stats: UserDashboardStats
  trend: TrendDataPoint[]
  models: ModelStat[]
  apiKeys: ApiKey[]
  dailyCheckinStatus: DailyCheckinStatus
  balance: number
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

const createApiKey = (overrides: Partial<ApiKey> & Pick<ApiKey, 'id' | 'key' | 'name'>): ApiKey => ({
  user_id: 1,
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: new Date().toISOString(),
  last_used_ip: '127.0.0.1',
  quota: 100,
  quota_used: 0,
  expires_at: null,
  created_at: new Date().toISOString(),
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
  ...overrides
})

export function createDashboardPreviewData(): DashboardPreviewData {
  const now = new Date()
  const trendValues = [
    [18420000, 6280000, 420000, 9480000],
    [21100000, 7140000, 510000, 10820000],
    [19680000, 6810000, 460000, 10340000],
    [23860000, 8250000, 590000, 12410000],
    [26740000, 9140000, 640000, 13980000],
    [24920000, 8560000, 610000, 13120000],
    [29180000, 9980000, 720000, 15360000]
  ]

  const trend = trendValues.map((values, index): TrendDataPoint => {
    const [inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens] = values
    const totalTokens = inputTokens + outputTokens + cacheCreationTokens + cacheReadTokens
    return {
      date: localDate(addDays(now, index - trendValues.length + 1)),
      requests: 12840 + index * 1370,
      input_tokens: inputTokens,
      output_tokens: outputTokens,
      cache_creation_tokens: cacheCreationTokens,
      cache_read_tokens: cacheReadTokens,
      total_tokens: totalTokens,
      cost: 82.4 + index * 9.7,
      actual_cost: 58.8 + index * 6.9
    }
  })

  return {
    balance: 184.62,
    stats: {
      total_api_keys: 4,
      active_api_keys: 3,
      total_requests: 1_248_392,
      total_input_tokens: 287_400_000,
      total_output_tokens: 104_900_000,
      total_cache_creation_tokens: 8_600_000,
      total_cache_read_tokens: 81_100_000,
      total_tokens: 482_000_000,
      total_cost: 1_214.67,
      total_actual_cost: 863.42,
      today_requests: 21_846,
      today_input_tokens: 29_180_000,
      today_output_tokens: 9_980_000,
      today_cache_creation_tokens: 720_000,
      today_cache_read_tokens: 15_360_000,
      today_tokens: 55_240_000,
      today_cost: 140.6,
      today_actual_cost: 100.2,
      average_duration_ms: 42,
      rpm: 286,
      tpm: 921_000,
      by_platform: []
    },
    trend,
    models: [
      {
        model: 'gpt-4.1',
        requests: 438_240,
        input_tokens: 103_600_000,
        output_tokens: 38_400_000,
        cache_creation_tokens: 2_100_000,
        cache_read_tokens: 28_900_000,
        total_tokens: 173_000_000,
        cost: 436.12,
        actual_cost: 302.81
      },
      {
        model: 'claude-sonnet-4',
        requests: 326_180,
        input_tokens: 82_400_000,
        output_tokens: 30_600_000,
        cache_creation_tokens: 3_800_000,
        cache_read_tokens: 24_200_000,
        total_tokens: 141_000_000,
        cost: 382.6,
        actual_cost: 271.34
      },
      {
        model: 'gemini-2.5-pro',
        requests: 284_706,
        input_tokens: 61_800_000,
        output_tokens: 21_200_000,
        cache_creation_tokens: 1_700_000,
        cache_read_tokens: 18_300_000,
        total_tokens: 103_000_000,
        cost: 236.45,
        actual_cost: 172.92
      },
      {
        model: 'grok-4',
        requests: 199_266,
        input_tokens: 39_600_000,
        output_tokens: 14_700_000,
        cache_creation_tokens: 1_000_000,
        cache_read_tokens: 9_700_000,
        total_tokens: 65_000_000,
        cost: 159.5,
        actual_cost: 116.35
      }
    ],
    apiKeys: [
      createApiKey({
        id: 101,
        key: 'sk-preview-prod-backend-7f2ca981',
        name: 'Production Backend',
        quota: 500,
        quota_used: 378.45
      }),
      createApiKey({
        id: 102,
        key: 'sk-preview-staging-43b7e210',
        name: 'Staging Environment',
        quota: 200,
        quota_used: 84.2,
        expires_at: addDays(now, 120).toISOString()
      }),
      createApiKey({
        id: 103,
        key: 'sk-preview-automation-a419c872',
        name: 'Automation Runner',
        quota: 0,
        quota_used: 28.7
      })
    ],
    dailyCheckinStatus: {
      enabled: true,
      ads_enabled: false,
      checked_in_today: true,
      today_reward: 0.35,
      recharge_eligible: true,
      checkin_date: localDate(now),
      last_checkin_at: now.toISOString(),
      next_available_at: addDays(now, 1).toISOString(),
      exhausted_today: false
    }
  }
}
