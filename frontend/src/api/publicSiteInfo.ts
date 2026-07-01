import { apiClient } from './client'
import type { MonitorStatus, Provider } from './admin/channelMonitor'

export interface PublicSiteGroupRate {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  allow_image_generation: boolean
  image_rate_multiplier: number
}

export interface PublicSiteMonitorTimelinePoint {
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  checked_at: string
}

export interface PublicSiteModelAvailability {
  model: string
  latest_status: MonitorStatus
  availability_7d: number
  availability_15d: number
  availability_30d: number
}

export interface PublicSiteMonitorAvailability {
  id: number
  name: string
  provider: Provider | string
  group_name: string
  models: PublicSiteModelAvailability[]
  timeline: PublicSiteMonitorTimelinePoint[]
}

export interface PublicSiteRechargeInfo {
  payment_enabled: boolean
  balance_disabled: boolean
  balance_recharge_multiplier: number
}

export interface PublicSiteInfo {
  generated_at: string
  groups: PublicSiteGroupRate[]
  model_availability: PublicSiteMonitorAvailability[]
  recharge: PublicSiteRechargeInfo
}

export async function getPublicSiteInfo(options?: { signal?: AbortSignal }): Promise<PublicSiteInfo> {
  const { data } = await apiClient.get<PublicSiteInfo>('/public/site-info', {
    signal: options?.signal,
  })
  return data
}

export const publicSiteInfoAPI = {
  getPublicSiteInfo,
}

export default publicSiteInfoAPI
