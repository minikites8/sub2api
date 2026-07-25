import type { ApiKey, Group, PublicSettings } from '@/types'

export interface KeysPreviewData {
  apiKeys: ApiKey[]
  groups: Group[]
  publicSettings: Partial<PublicSettings>
}

const minutesAgo = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()
const daysAgo = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString()

const createGroup = (
  id: number,
  name: string,
  platform: Group['platform'],
  rateMultiplier: number
) => ({
  id,
  name,
  description: `${name} routing group`,
  platform,
  rate_multiplier: rateMultiplier,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: true,
  allow_batch_image_generation: false,
  peak_rate_enabled: false,
  peak_start: '00:00',
  peak_end: '00:00',
  peak_rate_multiplier: 1,
  sort_order: id
}) as unknown as Group

const createApiKey = (
  group: Group,
  overrides: Partial<ApiKey> & Pick<ApiKey, 'id' | 'key' | 'name'>
): ApiKey => ({
  user_id: 1,
  group_id: group.id,
  group,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: minutesAgo(10),
  last_used_ip: '203.0.113.24',
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: daysAgo(94),
  updated_at: minutesAgo(10),
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

export function createKeysPreviewData(): KeysPreviewData {
  const gatewayGroup = createGroup(1, '统一网关', 'openai', 1)
  const claudeGroup = createGroup(2, 'Claude 高性能', 'anthropic', 1.2)
  const imageGroup = createGroup(3, '图像生成', 'openai', 1.5)
  const groups = [gatewayGroup, claudeGroup, imageGroup]

  return {
    groups,
    apiKeys: [
      createApiKey(gatewayGroup, {
        id: 101,
        key: 'sk-live-a1b2c3d4e5f6g7h8i9j0',
        name: '生产环境网关',
        ip_whitelist: ['10.0.0.0/8'],
        last_used_at: minutesAgo(10)
      }),
      createApiKey(claudeGroup, {
        id: 102,
        key: 'sk-test-x9y8z7w6v5u4t3s2r1',
        name: '开发测试节点',
        created_at: daysAgo(41),
        last_used_at: minutesAgo(125)
      }),
      createApiKey(imageGroup, {
        id: 103,
        key: 'sk-img-k8m2p9q4r7v1w5z3c6n0',
        name: '图像生成服务',
        created_at: daysAgo(18),
        last_used_at: daysAgo(2)
      }),
      createApiKey(gatewayGroup, {
        id: 104,
        key: 'sk-legacy-f4e3d2c1b0a987654321',
        name: '旧版集成',
        status: 'inactive',
        created_at: daysAgo(187),
        last_used_at: daysAgo(63)
      })
    ],
    publicSettings: {
      site_name: 'your-code.cc',
      api_base_url: 'https://api.your-code.cc',
      custom_endpoints: [],
      hide_ccs_import_button: false
    }
  }
}
