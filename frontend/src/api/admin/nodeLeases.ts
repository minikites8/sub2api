import { apiClient } from '../client'

export type QuotaLeaseTaskStatus =
  | 'pending'
  | 'waiting_callback'
  | 'callback_ready'
  | 'completed'
  | 'failed'

export interface NodeLeaseControlOptions {
  signal?: AbortSignal
}

const nodeLeaseAdminBase = '/admin/node-leases'

export interface QuotaLeaseNode {
  node_id: string
  region?: string
  base_url?: string
  public_key?: string
  metadata?: Record<string, string>
  status: string
  inflight_requests: number
  lease_remaining: number
  metrics?: Record<string, number>
  sync_status?: QuotaLeaseNodeSyncStatus
  registered_at: string
  last_heartbeat_at?: string
  updated_at: string
}

export interface QuotaLeaseNodeSyncStatus {
  mirror_ready: boolean
  mirror_synced_at?: string
  last_sync_started_at?: string
  last_sync_success_at?: string
  last_sync_failed_at?: string
  last_sync_error?: string
  last_sync_mode?: string
  mirror_version: number
  synced_group_count: number
  synced_channel_count: number
  synced_proxy_count: number
  synced_account_count: number
  pending_usage_events: number
  pending_usage_logs: number
  pending_ops_error_logs: number
}

export interface RegisterNodeRequest {
  node_id?: string
  region?: string
  base_url?: string
  public_key?: string
  metadata?: Record<string, string>
}

export interface UpdateNodeRequest {
  region?: string
  base_url?: string
  public_key?: string
  metadata?: Record<string, string>
  status?: string
}

export interface RegisterNodeResult {
  node: QuotaLeaseNode
  node_secret: string
}

export interface CreateNodeRegistrationURLRequest extends RegisterNodeRequest {
  ttl_seconds?: number
}

export interface NodeRegistrationURLResult {
  registration_url: string
  node_id?: string
  expires_at: string
  created_at: string
}

export interface QuotaLeaseSettings {
  prefetch_low_watermark_amount: number
  prefetch_average_window: number
  prefetch_average_multiplier: number
  prefetch_debounce_seconds: number
}

export interface QuotaLeaseLease {
  id: string
  node_id: string
  user_id: number
  api_key_id: number
  granted: number
  consumed: number
  reclaimed: number
  status: string
  expires_at: string
  reclaim_at: string
  created_at: string
  updated_at: string
}

export interface QuotaLeaseLedgerEvent {
  event_id: string
  lease_id: string
  node_id: string
  user_id: number
  api_key_id: number
  request_id: string
  amount: number
  event_type: string
  payload_hash: string
  created_at: string
}

export interface QuotaLeaseSnapshotStats {
  active_leases: number
  expired_leases: number
  closed_leases: number
  reclaimed_leases: number
  granted_total: number
  consumed_total: number
  reclaimed_total: number
  remaining_total: number
  event_count: number
  node_count: number
  online_nodes: number
}

export interface QuotaLeaseSnapshot {
  enabled: boolean
  node_id: string
  nodes: QuotaLeaseNode[]
  leases: QuotaLeaseLease[]
  events: QuotaLeaseLedgerEvent[]
  stats: QuotaLeaseSnapshotStats
}

export interface QuotaLeaseAccountSnapshot {
  id: number
  name: string
  platform: string
  type: string
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  status: string
  error_message?: string
  schedulable: boolean
  concurrency: number
  priority: number
  group_ids?: number[]
  expires_at?: string
  rate_limit_reset_at?: string
  temp_unschedulable_until?: string
  temp_unschedulable_reason?: string
  updated_at: string
}

export interface QuotaLeaseAccountLoginTask {
  id: string
  account_id: number
  name: string
  platform: string
  type: string
  assigned_node_id: string
  login_payload?: Record<string, unknown>
  metadata?: Record<string, string>
  group_ids?: number[]
  concurrency: number
  priority: number
  status: QuotaLeaseTaskStatus
  error?: string
  account?: QuotaLeaseAccountSnapshot
  created_at: string
  updated_at: string
  completed_at?: string
}

export interface CreateAccountLoginTaskRequest {
  account_id: number
  name?: string
  platform: string
  type: string
  assigned_node_id: string
  login_payload?: Record<string, unknown>
  metadata?: Record<string, string>
  group_ids?: number[]
  concurrency?: number
  priority?: number
}

export interface SubmitAccountLoginTaskCallbackRequest {
  code?: string
  state?: string
  session_id?: string
  redirect_uri?: string
  callback_url?: string
  proxy_id?: number
  payload?: Record<string, unknown>
}

export interface QuotaLeaseAssignedAccount {
  node_id: string
  task_id?: string
  account: QuotaLeaseAccountSnapshot
  created_at: string
  updated_at: string
}

export interface QuotaLeaseReclaimResult {
  expired_count: number
  reclaimed_count: number
  reclaimed_total: number
}

export type QuotaLeaseDiagnosticHealth = 'ok' | 'warning' | 'critical'

export interface QuotaLeaseDiagnosticStats {
  node_count: number
  online_nodes: number
  user_count: number
  lease_count: number
  active_leases: number
  expired_leases: number
  closed_leases: number
  reclaimed_leases: number
  overdraft_leases: number
  low_capacity_leases: number
  granted_total: number
  consumed_total: number
  reclaimed_total: number
  remaining_total: number
  overdraft_total: number
  event_count: number
  pending_usage_events: number
  pending_usage_logs: number
  pending_ops_error_logs: number
  issue_count: number
  warning_count: number
  critical_count: number
}

export interface QuotaLeaseDiagnosticIssue {
  id: string
  level: QuotaLeaseDiagnosticHealth
  scope: string
  code: string
  message: string
  detail?: string
  node_id?: string
  user_id?: number
  api_key_id?: number
  lease_id?: string
  created_at?: string
}

export interface QuotaLeaseNodeDiagnostic {
  node_id: string
  region?: string
  base_url?: string
  status: string
  health: QuotaLeaseDiagnosticHealth
  issues?: string[]
  last_heartbeat_at?: string
  heartbeat_age_seconds?: number
  active_lease_count: number
  active_remaining: number
  overdraft_amount: number
  lease_count: number
  sync_status?: QuotaLeaseNodeSyncStatus
  pending_usage_events: number
  pending_usage_logs: number
  pending_ops_error_logs: number
}

export interface QuotaLeaseUserDiagnostic {
  user_id: number
  username?: string
  email?: string
  status?: string
  balance?: number
  frozen_balance?: number
  profile_error?: string
  health: QuotaLeaseDiagnosticHealth
  issues?: string[]
  api_key_ids?: number[]
  lease_count: number
  active_lease_count: number
  active_remaining: number
  overdraft_amount: number
  granted_total: number
  consumed_total: number
  reclaimed_total: number
  last_lease_at?: string
  last_event_at?: string
}

export interface QuotaLeaseLeaseDiagnostic {
  id: string
  node_id: string
  user_id: number
  api_key_id: number
  status: string
  health: QuotaLeaseDiagnosticHealth
  issues?: string[]
  granted: number
  consumed: number
  reclaimed: number
  remaining: number
  event_count: number
  usage_event_total: number
  last_event_at?: string
  expires_at: string
  reclaim_at: string
  created_at: string
  updated_at: string
  expires_in_seconds: number
  reclaim_in_seconds: number
}

export interface QuotaLeaseDiagnostics {
  generated_at: string
  enabled: boolean
  node_id: string
  health: QuotaLeaseDiagnosticHealth
  default_grant_amount: number
  preflight_reserve_amount: number
  stats: QuotaLeaseDiagnosticStats
  issues: QuotaLeaseDiagnosticIssue[]
  nodes: QuotaLeaseNodeDiagnostic[]
  users: QuotaLeaseUserDiagnostic[]
  leases: QuotaLeaseLeaseDiagnostic[]
}

function requestConfig(options?: NodeLeaseControlOptions) {
  return {
    signal: options?.signal
  }
}

export async function getStatus(options?: NodeLeaseControlOptions): Promise<QuotaLeaseSnapshot> {
  const { data } = await apiClient.get<QuotaLeaseSnapshot>(
    `${nodeLeaseAdminBase}/status`,
    requestConfig(options)
  )
  return data
}

export async function listNodes(options?: NodeLeaseControlOptions): Promise<QuotaLeaseNode[]> {
  const { data } = await apiClient.get<{ nodes: QuotaLeaseNode[] }>(
    `${nodeLeaseAdminBase}/nodes`,
    requestConfig(options)
  )
  return data.nodes || []
}

export async function updateNode(
  nodeId: string,
  payload: UpdateNodeRequest,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseNode> {
  const { data } = await apiClient.put<{ node: QuotaLeaseNode }>(
    `${nodeLeaseAdminBase}/nodes/${encodeURIComponent(nodeId)}`,
    payload,
    requestConfig(options)
  )
  return data.node
}

export async function getSettings(options?: NodeLeaseControlOptions): Promise<QuotaLeaseSettings> {
  const { data } = await apiClient.get<QuotaLeaseSettings>(
    `${nodeLeaseAdminBase}/settings`,
    requestConfig(options)
  )
  return data
}

export async function updateSettings(
  payload: QuotaLeaseSettings,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseSettings> {
  const { data } = await apiClient.put<QuotaLeaseSettings>(
    `${nodeLeaseAdminBase}/settings`,
    payload,
    requestConfig(options)
  )
  return data
}

export async function registerNode(
  payload: RegisterNodeRequest,
  options?: NodeLeaseControlOptions
): Promise<RegisterNodeResult> {
  const { data } = await apiClient.post<RegisterNodeResult>(
    `${nodeLeaseAdminBase}/nodes/register`,
    payload,
    requestConfig(options)
  )
  return data
}

export async function createNodeRegistrationURL(
  payload: CreateNodeRegistrationURLRequest,
  options?: NodeLeaseControlOptions
): Promise<NodeRegistrationURLResult> {
  const { data } = await apiClient.post<NodeRegistrationURLResult>(
    `${nodeLeaseAdminBase}/nodes/registration-urls`,
    payload,
    requestConfig(options)
  )
  return data
}

export async function listLoginTasks(
  params?: { status?: string; node_id?: string },
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseAccountLoginTask[]> {
  const { data } = await apiClient.get<{ tasks: QuotaLeaseAccountLoginTask[] }>(
    `${nodeLeaseAdminBase}/accounts/login-tasks`,
    {
      ...requestConfig(options),
      params: {
        status: params?.status || undefined,
        node_id: params?.node_id || undefined
      }
    }
  )
  return data.tasks || []
}

export async function createLoginTask(
  payload: CreateAccountLoginTaskRequest,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseAccountLoginTask> {
  const { data } = await apiClient.post<{ task: QuotaLeaseAccountLoginTask }>(
    `${nodeLeaseAdminBase}/accounts/login-tasks`,
    payload,
    requestConfig(options)
  )
  return data.task
}

export async function submitLoginTaskCallback(
  taskId: string,
  payload: SubmitAccountLoginTaskCallbackRequest,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseAccountLoginTask> {
  const { data } = await apiClient.post<{ task: QuotaLeaseAccountLoginTask }>(
    `${nodeLeaseAdminBase}/accounts/login-tasks/${encodeURIComponent(taskId)}/callback`,
    payload,
    requestConfig(options)
  )
  return data.task
}

export async function listAssignedAccounts(
  params?: { node_id?: string },
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseAssignedAccount[]> {
  const { data } = await apiClient.get<{ accounts: QuotaLeaseAssignedAccount[] }>(
    `${nodeLeaseAdminBase}/accounts/assignments`,
    {
      ...requestConfig(options),
      params: {
        node_id: params?.node_id || undefined
      }
    }
  )
  return data.accounts || []
}

export async function reclaimExpired(
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseReclaimResult> {
  const { data } = await apiClient.post<QuotaLeaseReclaimResult>(
    `${nodeLeaseAdminBase}/reclaim`,
    {},
    requestConfig(options)
  )
  return data
}

export async function getDiagnostics(options?: NodeLeaseControlOptions): Promise<QuotaLeaseDiagnostics> {
  const { data } = await apiClient.get<{ diagnostics: QuotaLeaseDiagnostics }>(
    `${nodeLeaseAdminBase}/diagnostics`,
    requestConfig(options)
  )
  return data.diagnostics
}

export const nodeLeasesAPI = {
  getStatus,
  listNodes,
  updateNode,
  getSettings,
  updateSettings,
  registerNode,
  createNodeRegistrationURL,
  listLoginTasks,
  createLoginTask,
  submitLoginTaskCallback,
  listAssignedAccounts,
  reclaimExpired,
  getDiagnostics
}

export default nodeLeasesAPI
