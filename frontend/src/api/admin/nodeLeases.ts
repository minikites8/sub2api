import { apiClient } from '../client'

export type QuotaLeaseDemoTaskStatus =
  | 'pending'
  | 'waiting_callback'
  | 'callback_ready'
  | 'completed'
  | 'failed'

export interface NodeLeaseControlOptions {
  signal?: AbortSignal
}

const nodeLeaseDemoAdminBase = '/admin/node-leases/demo'

export interface QuotaLeaseDemoNode {
  node_id: string
  region?: string
  base_url?: string
  public_key?: string
  metadata?: Record<string, string>
  status: string
  inflight_requests: number
  lease_remaining: number
  metrics?: Record<string, number>
  sync_status?: QuotaLeaseDemoNodeSyncStatus
  registered_at: string
  last_heartbeat_at?: string
  updated_at: string
}

export interface QuotaLeaseDemoNodeSyncStatus {
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
  node: QuotaLeaseDemoNode
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

export interface QuotaLeaseDemoSettings {
  prefetch_low_watermark_amount: number
  prefetch_average_window: number
  prefetch_average_multiplier: number
  prefetch_debounce_seconds: number
}

export interface QuotaLeaseDemoLease {
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

export interface QuotaLeaseDemoLedgerEvent {
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

export interface QuotaLeaseDemoSnapshotStats {
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

export interface QuotaLeaseDemoSnapshot {
  enabled: boolean
  node_id: string
  nodes: QuotaLeaseDemoNode[]
  leases: QuotaLeaseDemoLease[]
  events: QuotaLeaseDemoLedgerEvent[]
  stats: QuotaLeaseDemoSnapshotStats
}

export interface QuotaLeaseDemoAccountSnapshot {
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

export interface QuotaLeaseDemoAccountLoginTask {
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
  status: QuotaLeaseDemoTaskStatus
  error?: string
  account?: QuotaLeaseDemoAccountSnapshot
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

export interface QuotaLeaseDemoAssignedAccount {
  node_id: string
  task_id?: string
  account: QuotaLeaseDemoAccountSnapshot
  created_at: string
  updated_at: string
}

export interface QuotaLeaseDemoReclaimResult {
  expired_count: number
  reclaimed_count: number
  reclaimed_total: number
}

function requestConfig(options?: NodeLeaseControlOptions) {
  return {
    signal: options?.signal
  }
}

export async function getStatus(options?: NodeLeaseControlOptions): Promise<QuotaLeaseDemoSnapshot> {
  const { data } = await apiClient.get<QuotaLeaseDemoSnapshot>(
    `${nodeLeaseDemoAdminBase}/status`,
    requestConfig(options)
  )
  return data
}

export async function listNodes(options?: NodeLeaseControlOptions): Promise<QuotaLeaseDemoNode[]> {
  const { data } = await apiClient.get<{ nodes: QuotaLeaseDemoNode[] }>(
    `${nodeLeaseDemoAdminBase}/nodes`,
    requestConfig(options)
  )
  return data.nodes || []
}

export async function updateNode(
  nodeId: string,
  payload: UpdateNodeRequest,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseDemoNode> {
  const { data } = await apiClient.put<{ node: QuotaLeaseDemoNode }>(
    `${nodeLeaseDemoAdminBase}/nodes/${encodeURIComponent(nodeId)}`,
    payload,
    requestConfig(options)
  )
  return data.node
}

export async function getSettings(options?: NodeLeaseControlOptions): Promise<QuotaLeaseDemoSettings> {
  const { data } = await apiClient.get<QuotaLeaseDemoSettings>(
    `${nodeLeaseDemoAdminBase}/settings`,
    requestConfig(options)
  )
  return data
}

export async function updateSettings(
  payload: QuotaLeaseDemoSettings,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseDemoSettings> {
  const { data } = await apiClient.put<QuotaLeaseDemoSettings>(
    `${nodeLeaseDemoAdminBase}/settings`,
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
    `${nodeLeaseDemoAdminBase}/nodes/register`,
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
    `${nodeLeaseDemoAdminBase}/nodes/registration-urls`,
    payload,
    requestConfig(options)
  )
  return data
}

export async function listLoginTasks(
  params?: { status?: string; node_id?: string },
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseDemoAccountLoginTask[]> {
  const { data } = await apiClient.get<{ tasks: QuotaLeaseDemoAccountLoginTask[] }>(
    `${nodeLeaseDemoAdminBase}/accounts/login-tasks`,
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
): Promise<QuotaLeaseDemoAccountLoginTask> {
  const { data } = await apiClient.post<{ task: QuotaLeaseDemoAccountLoginTask }>(
    `${nodeLeaseDemoAdminBase}/accounts/login-tasks`,
    payload,
    requestConfig(options)
  )
  return data.task
}

export async function submitLoginTaskCallback(
  taskId: string,
  payload: SubmitAccountLoginTaskCallbackRequest,
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseDemoAccountLoginTask> {
  const { data } = await apiClient.post<{ task: QuotaLeaseDemoAccountLoginTask }>(
    `${nodeLeaseDemoAdminBase}/accounts/login-tasks/${encodeURIComponent(taskId)}/callback`,
    payload,
    requestConfig(options)
  )
  return data.task
}

export async function listAssignedAccounts(
  params?: { node_id?: string },
  options?: NodeLeaseControlOptions
): Promise<QuotaLeaseDemoAssignedAccount[]> {
  const { data } = await apiClient.get<{ accounts: QuotaLeaseDemoAssignedAccount[] }>(
    `${nodeLeaseDemoAdminBase}/accounts/assignments`,
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
): Promise<QuotaLeaseDemoReclaimResult> {
  const { data } = await apiClient.post<QuotaLeaseDemoReclaimResult>(
    `${nodeLeaseDemoAdminBase}/reclaim`,
    {},
    requestConfig(options)
  )
  return data
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
  reclaimExpired
}

export default nodeLeasesAPI
