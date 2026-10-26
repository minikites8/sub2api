import { apiClient } from '../client'
export type QoSTier = 'premium' | 'standard' | 'economy'
export interface QoSGroup {
  qos_tier: QoSTier; weight: number; allow_basic_fallback: boolean; strong_wait_ms: number; basic_wait_ms: number
  max_waiting: number; user_max_waiting: number; capacity_protection: boolean; concurrency: number
}
export interface QoSConfig {
  group_ids?: number[]; version: number; quality_routing_enabled: boolean; enabled: boolean; capacity_enabled: boolean
  premium_weight: number; standard_weight: number; economy_weight: number; max_waiting: number
  request_budget_ms: number; high_effort_budget_ms: number; max_attempts: number; log_sample_rate: number
  peak: { enabled: boolean; enter_utilization: number; exit_utilization: number; protection_ratio: number; enter_hold_ms: number; exit_hold_ms: number; strict_reserve: boolean }
  groups: Record<string, QoSGroup>; resource_pools: Record<string, { concurrency: number }>
}
export interface QoSTierMetrics {
  Requests: number; Attempts: number; Successes: number; FirstSuccesses: number; Canceled: number
  Downgrades: number; StrongRequests: number; VM2APIRequests: number; DirectRequests: number
  strong_hit_rate: number; first_success_rate: number; average_queue_ms: number; p50_queue_ms: number; p95_queue_ms: number; p99_queue_ms: number
}
export interface QoSPoolState { id: string; capacity: number; active: number; strong_capacity: number; strong_active: number; peak: boolean; reserved: number; premium_active: number; premium_reserved_used: number; reason: string }
export interface QoSSnapshot { available: boolean; config?: QoSConfig; capacity?: { queued: number; active: number; pools: QoSPoolState[]; strong_pool?: Partial<QoSPoolState> }; tiers: Record<string, QoSTierMetrics>; metrics_window: string; metrics_dropped: number; downgrade_reasons?: Record<string, number>; upstream_requests?: Record<string, number> }
export const defaultQoSGroup = (qos_tier: QoSTier = 'standard'): QoSGroup => ({ qos_tier, weight: { premium: 6, standard: 3, economy: 1 }[qos_tier], strong_wait_ms: { premium: 5000, standard: 2000, economy: 500 }[qos_tier], basic_wait_ms: 1000, allow_basic_fallback: qos_tier === 'economy', capacity_protection: qos_tier === 'premium', max_waiting: 200, user_max_waiting: 4, concurrency: 0 })
export async function getQoSConfig(): Promise<QoSConfig> { return (await apiClient.get('/admin/gpt-qos/config')).data }
export async function saveQoSConfig(config: QoSConfig): Promise<QoSConfig> { return (await apiClient.put('/admin/gpt-qos/config', config)).data }
export async function getQoSSnapshot(params: Record<string, string | number> = {}): Promise<QoSSnapshot> { return (await apiClient.get('/admin/gpt-qos/snapshot', { params })).data }
