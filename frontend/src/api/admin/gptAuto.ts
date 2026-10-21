import { apiClient } from '../client'

// GPT-Auto Phase 1: shadow observation only. Nothing here changes live routing
// or billing.

export type GPTAutoEndpoint = 'chat_completions' | 'responses'
export type GPTAutoEvaluationStatus = 'success' | 'failed'
export type GPTAutoDecisionStatus = 'accepted' | 'adjusted' | 'no_eligible_candidate'

export interface GPTAutoUpstreamCost {
  channel: string
  enabled: boolean
  input_per_m: number
  output_per_m: number
  cached_input_per_m: number
}

export interface GPTAutoModelProfile {
  model: string
  description: string
  enabled: boolean
  billing_multiplier: number
  routing_weight: number
  supported_efforts: string[]
  supports_tools: boolean
  endpoints?: GPTAutoEndpoint[]
  min_margin?: number | null
  upstream_costs?: GPTAutoUpstreamCost[]
}

export interface GPTAutoConfig {
  shadow_enabled: boolean
  sample_rate: number
  endpoints: GPTAutoEndpoint[]
  all_groups: boolean
  group_ids: number[] | null
  model_filter: string[] | null
  classifier_base_url: string
  classifier_proxy_id?: number | null
  classifier_model: string
  classifier_timeout_ms: number
  classifier_max_concurrency: number
  classifier_retry_count: number
  classifier_daily_call_limit: number
  classifier_daily_cost_limit: number
  classification_max_context_chars: number
  classification_context_messages: number
  worker_count: number
  queue_size: number
  decision_retention_days: number
  global_min_margin: number
  profit_safety_buffer: number
  model_profiles: GPTAutoModelProfile[]
  policy_version: string
  classifier_api_key_configured: boolean
  classifier_api_key_masked: string
}

export interface UpdateGPTAutoConfig extends Omit<GPTAutoConfig, 'classifier_api_key_configured' | 'classifier_api_key_masked'> {
  /** Empty keeps the stored key. */
  classifier_api_key?: string
  clear_classifier_api_key?: boolean
}

export interface GPTAutoMeta {
  models: { model: string; native_supported_efforts: string[] }[]
  efforts: string[]
  categories: string[]
  endpoints: GPTAutoEndpoint[]
  default_profiles: GPTAutoModelProfile[]
  policy_version: string
  phase: string
  live_routing_active: boolean
}

export interface GPTAutoCount {
  key: string
  count: number
}

export interface GPTAutoPairCount {
  actual: string
  recommended: string
  count: number
}

export interface GPTAutoTrendPoint {
  bucket: string
  success: number
  failed: number
}

export interface GPTAutoStoredStats {
  total: number
  success: number
  failed: number
  avg_latency_ms?: number
  p50_latency_ms?: number
  p95_latency_ms?: number
  input_tokens: number
  output_tokens: number
  classifier_cost: number
  adjusted: number
  no_eligible: number
  below_target_count: number
  final_models: GPTAutoCount[]
  jev_models: GPTAutoCount[]
  final_efforts: GPTAutoCount[]
  complexity: GPTAutoCount[]
  categories: GPTAutoCount[]
  error_classes: GPTAutoCount[]
  actual_vs_final: GPTAutoPairCount[]
  trend: GPTAutoTrendPoint[]
  trend_bucket_size: 'hour' | 'day'
}

export interface GPTAutoRuntimeStatus {
  /** This node since process start. */
  counters: Record<string, number>
  queue_length: number
  queue_capacity: number
  queue_bytes: number
  active_calls: number
  /** This node's breaker. */
  circuit_state: 'closed' | 'open' | 'half_open'
  /** Cluster-wide UTC-day budget totals. */
  today_calls: number
  today_cost: number
  budget_scope: 'cluster' | 'unavailable'
  /** All nodes' counters for the UTC day, as of each node's last flush. */
  cluster_counters_today?: Record<string, number>
  recent_p50_ms?: number
  recent_p95_ms?: number
  recent_latency_samples: number
  excluded_endpoints: string[]
}

export interface GPTAutoOverview {
  shadow_enabled: boolean
  sample_rate: number
  classifier_configured: boolean
  policy_version: string
  start_time: string
  end_time: string
  stored: GPTAutoStoredStats
  runtime: GPTAutoRuntimeStatus
  error_rate?: number
}

export interface GPTAutoClassification {
  classifier_model: string
  model: string
  model_confidence: number
  model_probabilities?: Record<string, number>
  effort: string
  effort_confidence: number
  effort_probabilities?: Record<string, number>
  complexity: number
  complexity_confidence: number
  complexity_probabilities?: Record<string, number>
  category: string
  category_confidence: number
  category_probabilities?: Record<string, number>
  input_tokens: number
  output_tokens: number
}

export interface GPTAutoAdjustment {
  rule: string
  from?: string
  to?: string
  detail?: string
}

export interface GPTAutoCandidate {
  model: string
  eligible: boolean
  reasons?: string[]
  probability: number
  routing_weight: number
}

export interface GPTAutoEconomics {
  hypothetical: boolean
  model: string
  effort: string
  billing_multiplier: number
  estimated_input_tokens: number
  estimated_output_tokens: number
  output_estimate_source: string
  output_estimate_samples: number
  list_input_per_m: number
  list_output_per_m: number
  upstream_channel?: string
  upstream_input_per_m: number
  upstream_output_per_m: number
  upstream_cost_source: string
  estimated_revenue: number
  estimated_upstream_cost: number
  estimated_classifier_cost: number
  estimated_gross_profit: number
  estimated_gross_margin?: number
  target_margin: number
  min_required_multiplier?: number
  below_target: boolean
  pricing_available: boolean
}

export interface GPTAutoPolicyDecision {
  policy_version: string
  status: GPTAutoDecisionStatus
  jev_model: string
  jev_effort: string
  final_model?: string
  final_effort?: string
  candidates: GPTAutoCandidate[]
  adjustments?: GPTAutoAdjustment[]
  capability_warnings?: string[]
  economics?: GPTAutoEconomics
}

export interface GPTAutoDecision {
  id: number
  created_at: string
  request_id: string
  user_id?: number
  api_key_id?: number
  group_id?: number
  endpoint: GPTAutoEndpoint
  actual_model: string
  actual_effort: string
  stream: boolean
  has_tools: boolean
  evaluation_status: GPTAutoEvaluationStatus
  error_class?: string
  decision_status?: GPTAutoDecisionStatus
  jev_model: string
  jev_effort: string
  final_model: string
  final_effort: string
  complexity?: number
  category: string
  model_confidence?: number
  effort_confidence?: number
  classifier_model: string
  classifier_input_tokens: number
  classifier_output_tokens: number
  classifier_cost: number
  classifier_latency_ms: number
  queue_delay_ms: number
  attempts: number
  est_revenue?: number
  est_upstream_cost?: number
  est_gross_profit?: number
  est_gross_margin?: number
  margin_below_target: boolean
  policy_version: string
  policy_snapshot_hash: string
  features: { endpoint: string; has_tools: boolean; estimated_input_tokens: number }
  classification?: GPTAutoClassification
  decision?: GPTAutoPolicyDecision
  /** Real production outcome from usage_logs; detail endpoint only. */
  actual_usage?: GPTAutoActualUsage
}

export interface GPTAutoActualUsage {
  model: string
  requested_model: string
  upstream_model: string
  reasoning_effort: string
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  list_cost: number
  charged_cost: number
  account_cost: number
  created_at: string
}

export interface ListGPTAutoDecisionsParams {
  page?: number
  page_size?: number
  start_time?: string
  end_time?: string
  actual_model?: string
  final_model?: string
  final_effort?: string
  evaluation_status?: GPTAutoEvaluationStatus | ''
  decision_status?: GPTAutoDecisionStatus | ''
  group_id?: number
  request_id?: string
}

export interface GPTAutoDecisionsResponse {
  items: GPTAutoDecision[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface GPTAutoContextTurn {
  role: string
  text: string
}

export interface GPTAutoTestInput {
  prompt: string
  context: GPTAutoContextTurn[]
  endpoint: GPTAutoEndpoint
  has_tools: boolean
  model_profiles?: GPTAutoModelProfile[]
  estimated_input_tokens?: number
}

export interface GPTAutoTestResult {
  classification: GPTAutoClassification
  decision: GPTAutoPolicyDecision
  classifier_cost: number
  latency_ms: number
  snapshot_preview: string
  profiles_overridden: boolean
}

export type GPTAutoReplayMode = 'historical' | 'current' | 'custom'

export interface GPTAutoReplayInput {
  mode: GPTAutoReplayMode
  model_profiles?: GPTAutoModelProfile[]
  global_min_margin?: number
  profit_safety_buffer?: number
}

export interface GPTAutoReplayResult {
  decision_id: number
  mode: GPTAutoReplayMode
  original?: GPTAutoPolicyDecision
  replayed: GPTAutoPolicyDecision
  changed: boolean
}

export interface GPTAutoSimulationInput {
  model_profiles: GPTAutoModelProfile[]
  global_min_margin?: number
  profit_safety_buffer?: number
  effort: string
  input_tokens: number
  output_tokens?: number
  include_classifier_cost: boolean
  classifier_input_tokens: number
}

export interface GPTAutoSimulationResult {
  rows: GPTAutoEconomics[]
  classifier_cost: number
}

const base = '/admin/gpt-auto'

export async function getMeta(): Promise<GPTAutoMeta> {
  const { data } = await apiClient.get<GPTAutoMeta>(`${base}/meta`)
  return data
}

export async function getConfig(): Promise<GPTAutoConfig> {
  const { data } = await apiClient.get<GPTAutoConfig>(`${base}/config`)
  return data
}

export async function updateConfig(payload: UpdateGPTAutoConfig): Promise<GPTAutoConfig> {
  const { data } = await apiClient.put<GPTAutoConfig>(`${base}/config`, payload)
  return data
}

export async function getOverview(params: { start_time?: string; end_time?: string; group_id?: number } = {}): Promise<GPTAutoOverview> {
  const { data } = await apiClient.get<GPTAutoOverview>(`${base}/overview`, { params })
  return data
}

export async function listDecisions(params: ListGPTAutoDecisionsParams = {}): Promise<GPTAutoDecisionsResponse> {
  const cleaned = Object.fromEntries(Object.entries(params).filter(([, value]) => value !== '' && value !== undefined && value !== null))
  const { data } = await apiClient.get<GPTAutoDecisionsResponse>(`${base}/decisions`, { params: cleaned })
  return data
}

export async function getDecision(id: number): Promise<GPTAutoDecision> {
  const { data } = await apiClient.get<GPTAutoDecision>(`${base}/decisions/${id}`)
  return data
}

export async function replayDecision(id: number, payload: GPTAutoReplayInput): Promise<GPTAutoReplayResult> {
  const { data } = await apiClient.post<GPTAutoReplayResult>(`${base}/decisions/${id}/replay`, payload)
  return data
}

export async function runTest(payload: GPTAutoTestInput): Promise<GPTAutoTestResult> {
  // Jev calls can take up to the configured classifier timeout (max 60s).
  const { data } = await apiClient.post<GPTAutoTestResult>(`${base}/test`, payload, { timeout: 70000 })
  return data
}

export async function simulate(payload: GPTAutoSimulationInput): Promise<GPTAutoSimulationResult> {
  const { data } = await apiClient.post<GPTAutoSimulationResult>(`${base}/simulate`, payload)
  return data
}

export const gptAutoAPI = {
  getMeta,
  getConfig,
  updateConfig,
  getOverview,
  listDecisions,
  getDecision,
  replayDecision,
  runTest,
  simulate,
}

export default gptAutoAPI
