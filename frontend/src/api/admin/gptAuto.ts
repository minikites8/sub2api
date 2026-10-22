import { apiClient } from '../client'

// GPT-Auto Phase 1: shadow observation only. Nothing here changes live routing
// or billing.

export type GPTAutoEndpoint = 'chat_completions' | 'responses'
export type GPTAutoContextStrategy = 'task_aware' | 'recent_turns'
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
  classification_context_strategy?: GPTAutoContextStrategy
  classification_agent_steps?: number
  worker_count: number
  queue_size: number
  decision_retention_days: number
  global_min_margin: number
  profit_safety_buffer: number
  model_profiles: GPTAutoModelProfile[]
  policy_version: string
  /** Read-only here: edited through the classifier endpoint. */
  classifier_instructions?: GPTAutoClassifierInstructions
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
  classifier_schema?: string
  state_version?: string
  diagnostics_version?: string
  context_strategies?: GPTAutoContextStrategy[]
  benchmark_samples?: GPTAutoBenchmarkSample[]
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
  /** Fully loaded margin below target (includes Jev cost). */
  below_target_count: number
  model_below_target_count?: number
  insufficient_estimates?: number
  policy_model_changes?: number
  policy_effort_changes?: number
  final_models: GPTAutoCount[]
  jev_models: GPTAutoCount[]
  final_efforts: GPTAutoCount[]
  jev_efforts?: GPTAutoCount[]
  classifier_configs?: GPTAutoCount[]
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

export type GPTAutoMarginStatus = 'unknown' | 'loss' | 'below_target' | 'meets_target'
export type GPTAutoEstimateQuality = 'historical' | 'admin_assumption' | 'heuristic' | 'insufficient_data'

/**
 * Hypothetical economics. Fields marked optional are absent on decisions
 * stored before the model / fully loaded split; `estimated_gross_margin` and
 * `below_target` have always been fully loaded (Jev cost included).
 */
export interface GPTAutoEconomics {
  hypothetical: boolean
  model: string
  effort: string
  billing_multiplier: number
  effort_billing_multiplier?: number
  customer_rate_multiplier?: number
  customer_rate_source?: string
  estimated_input_tokens: number
  input_estimate_source?: string
  estimated_cached_input_tokens?: number
  cached_input_estimate_source?: string
  estimated_output_tokens: number
  output_estimate_source: string
  output_estimate_samples: number
  estimate_quality?: GPTAutoEstimateQuality
  list_input_per_m: number
  list_cached_input_per_m?: number
  list_output_per_m: number
  upstream_channel?: string
  upstream_input_per_m: number
  upstream_cached_input_per_m?: number
  upstream_output_per_m: number
  upstream_cost_source: string
  estimated_revenue: number
  estimated_upstream_cost: number
  estimated_model_gross_profit?: number
  model_gross_margin?: number | null
  model_margin_status?: GPTAutoMarginStatus
  model_below_target?: boolean
  estimated_classifier_cost: number
  classifier_cost_source?: string
  estimated_gross_profit: number
  estimated_gross_margin?: number | null
  fully_loaded_margin_status?: GPTAutoMarginStatus
  target_margin: number
  min_required_multiplier?: number | null
  min_required_multiplier_fully_loaded?: number | null
  below_target: boolean
  pricing_available: boolean
}

export interface GPTAutoObservedEconomics {
  model: string
  charged_revenue: number
  account_cost: number
  classifier_cost: number
  model_gross_profit: number
  model_gross_margin?: number | null
  fully_loaded_profit: number
  fully_loaded_margin?: number | null
  effective_rate?: number | null
}

export interface GPTAutoPolicyDecision {
  policy_version: string
  status: GPTAutoDecisionStatus
  jev_model: string
  jev_effort: string
  final_model?: string
  final_effort?: string
  candidates: GPTAutoCandidate[]
  /** Rules that changed or withheld the recommendation. */
  adjustments?: GPTAutoAdjustment[]
  /** Informational only; never change the recommendation. */
  warnings?: GPTAutoAdjustment[]
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
  est_model_gross_margin?: number
  model_margin_below_target?: boolean
  estimate_quality?: GPTAutoEstimateQuality | ''
  policy_version: string
  policy_snapshot_hash: string
  /** Empty for decisions classified before classifier versioning (v1 questions). */
  classifier_config_hash?: string
  features: { endpoint: string; has_tools: boolean; estimated_input_tokens: number; input_estimate_source?: string; context?: GPTAutoContextInfo }
  classification?: GPTAutoClassification
  decision?: GPTAutoPolicyDecision
  /** Real production outcome from usage_logs; detail endpoint only. */
  actual_usage?: GPTAutoActualUsage
  observed_economics?: GPTAutoObservedEconomics
  actual_input_economics?: GPTAutoEconomics
  classifier_config?: GPTAutoClassifierConfigRecord
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

export interface GPTAutoWorkloadAssumptions {
  /** Uncached input tokens. */
  input_tokens?: number
  cached_input_tokens?: number
  output_tokens?: number
}

export interface GPTAutoTestInput {
  prompt: string
  context: GPTAutoContextTurn[]
  endpoint: GPTAutoEndpoint
  has_tools: boolean
  model_profiles?: GPTAutoModelProfile[]
  assumptions?: GPTAutoWorkloadAssumptions
}

export interface GPTAutoTestResult {
  classification: GPTAutoClassification
  decision: GPTAutoPolicyDecision
  classifier_cost: number
  classifier_input_tokens?: number
  classifier_output_tokens?: number
  classifier_config_hash?: string
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
  cached_input_tokens?: number
  include_classifier_cost: boolean
  classifier_input_tokens: number
}

export interface GPTAutoSimulationResult {
  rows: GPTAutoEconomics[]
  classifier_cost: number
}

export interface GPTAutoClassifierInstructions {
  model_selection: string
  effort_selection: string
  complexity_scoring: string
  category_classification: string
  revision?: number
  updated_at?: string
  updated_by?: number
}

export type GPTAutoInstructionField = 'model_selection' | 'effort_selection' | 'complexity_scoring' | 'category_classification'
export const gptAutoInstructionFields: GPTAutoInstructionField[] = ['model_selection', 'effort_selection', 'complexity_scoring', 'category_classification']

export interface GPTAutoQuestion {
  type: string
  instructions: string
  criteria?: Record<string, string> | string[]
}

export interface GPTAutoClassifierSnapshot {
  schema_version: string
  state_version: string
  classifier_model: string
  questions: Record<string, GPTAutoQuestion>
}

export interface GPTAutoClassifierConfigRecord {
  hash: string
  revision: number
  snapshot: GPTAutoClassifierSnapshot
  created_at: string
  created_by?: number
}

export interface GPTAutoClassifierView {
  instructions: GPTAutoClassifierInstructions
  defaults: GPTAutoClassifierInstructions
  is_default: boolean
  hash: string
  effective: GPTAutoClassifierSnapshot
  history: GPTAutoClassifierConfigRecord[]
  limits: { min_runes: number; max_runes: number }
  legacy_instructions: Record<string, string>
}

export interface GPTAutoClassifierPreview {
  hash: string
  effective: GPTAutoClassifierSnapshot
  same_as_saved: boolean
}

export interface GPTAutoCompareInput extends GPTAutoTestInput {
  draft_instructions: GPTAutoClassifierInstructions
}

export interface GPTAutoCompareSide {
  classifier_config_hash: string
  result?: GPTAutoTestResult
  error_code?: string
  error_message?: string
}

export interface GPTAutoCompareResult {
  saved: GPTAutoCompareSide
  draft: GPTAutoCompareSide
  jev_calls: number
  total_classifier_cost: number
  snapshot_preview: string
}

/** Extraction metadata: counts, budgets and limitations only, no content. */
export interface GPTAutoContextInfo {
  state_version: string
  strategy: GPTAutoContextStrategy
  items_total: number
  scanned_bytes: number
  latest_user_index: number
  latest_user_kind: 'instruction' | 'follow_up' | 'none'
  latest_user_truncated: boolean
  follow_up_anchor_index: number
  original_request_index: number
  context_turns_available: number
  context_turns_included: number
  agent_events_included: number
  agent_events_omitted: number
  budget_chars: number
  used_chars: number
  section_chars?: Record<string, number>
  limitations?: string[]
  /** Absent on decisions created before context diagnostics existed. */
  diagnostics?: GPTAutoContextDiagnostics
}

export type GPTAutoContextSection =
  | 'latest_user_request'
  | 'continues_request'
  | 'agent_activity'
  | 'original_request'
  | 'history_summary'
  | 'recent_context'
  | 'external_history'

export type GPTAutoSelectionStatus = 'included' | 'truncated' | 'budget_exhausted' | 'unavailable' | 'not_applicable'

/** One builder step, in the order the server ran it. */
export interface GPTAutoContextSelectionStep {
  section: GPTAutoContextSection
  status: GPTAutoSelectionStatus
  /** Budget characters consumed (agent activity includes a per-event overhead). */
  chars: number
  items?: number
  available?: number
  dropped?: number
}

/** Every request item is counted once; eligible items are split into represented / budget_dropped / not_selected. */
export interface GPTAutoContextItemCounts {
  eligible: number
  represented: number
  selected_messages: number
  budget_dropped: number
  budget_truncated: number
  not_selected: number
  privacy: number
  unsupported: number
  external_references: number
  compacted: number
  empty: number
  with_images: number
  conversation_length_known: boolean
}

export interface GPTAutoAgentDiagnostics {
  scope: string
  step_limit: number
  steps_detected: number
  tool_calls_detected: number
  tool_results_detected: number
  tool_failures_detected: number
  trailing_failures: number
  failure_summaries_included: number
  events_dropped_for_budget: number
  event_kinds?: Record<string, number>
  has_tool_result_context: boolean
  has_progress_note: boolean
}

export type GPTAutoCompleteness = 'sufficient' | 'partial' | 'important_unavailable' | 'unable_to_determine'

/** context-diagnostics-v1: counts, statuses and fixed codes only, never request content. */
export interface GPTAutoContextDiagnostics {
  diagnostics_version: string
  scan_status: 'scanned' | 'skipped_too_large'
  /** Absent when the request was not scanned (counts unknown, not zero). */
  items?: GPTAutoContextItemCounts
  selection?: GPTAutoContextSelectionStep[]
  context_truncated: boolean
  follow_up: {
    resolution: 'not_follow_up' | 'linked_request' | 'agent_activity_only' | 'unresolved' | 'recent_turns_only'
    short_message: boolean
    original_task_available: boolean
    agent_activity_available: boolean
  }
  agent?: GPTAutoAgentDiagnostics
  notes?: string[]
  completeness: GPTAutoCompleteness
  completeness_reasons?: string[]
  /** Length of the serialized Jev state; absent where it was not recorded. */
  state_chars?: number
}

export interface GPTAutoContextOptions {
  max_chars: number
  context_turns: number
  strategy: GPTAutoContextStrategy
  agent_steps: number
}

export interface GPTAutoDebugInput {
  endpoint: GPTAutoEndpoint
  /** A synthetic OpenAI-compatible request object. */
  request: Record<string, unknown>
  max_chars?: number
  context_turns?: number
  strategy?: GPTAutoContextStrategy
  agent_steps?: number
}

export interface GPTAutoDebugResult {
  options: GPTAutoContextOptions
  classifiable: boolean
  jev_request: { model: string; state: string; questions: Record<string, GPTAutoQuestion> }
  state_chars: number
  estimated_jev_input_tokens: number
  context: GPTAutoContextInfo
  effective: GPTAutoClassifierSnapshot
  classifier_config_hash: string
  estimated_input_tokens: number
}

export interface GPTAutoBenchmarkSample {
  id: string
  group: string
  prompt: string
  context?: GPTAutoContextTurn[]
  has_tools: boolean
  expected_tier: 'luna' | 'sol'
  expected_category: string
  expected_complexity: [number, number]
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

export async function getClassifier(): Promise<GPTAutoClassifierView> {
  const { data } = await apiClient.get<GPTAutoClassifierView>(`${base}/classifier`)
  return data
}

export async function updateClassifier(payload: GPTAutoClassifierInstructions): Promise<GPTAutoClassifierView> {
  const { data } = await apiClient.put<GPTAutoClassifierView>(`${base}/classifier`, payload)
  return data
}

export async function previewClassifier(payload: GPTAutoClassifierInstructions): Promise<GPTAutoClassifierPreview> {
  const { data } = await apiClient.post<GPTAutoClassifierPreview>(`${base}/classifier/preview`, payload)
  return data
}

export async function compareClassifier(payload: GPTAutoCompareInput): Promise<GPTAutoCompareResult> {
  // Two sequential Jev calls, each up to the classifier timeout (max 60s).
  const { data } = await apiClient.post<GPTAutoCompareResult>(`${base}/test/compare`, payload, { timeout: 130000 })
  return data
}

export async function debugState(payload: GPTAutoDebugInput): Promise<GPTAutoDebugResult> {
  const { data } = await apiClient.post<GPTAutoDebugResult>(`${base}/debug/state`, payload)
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
  getClassifier,
  updateClassifier,
  previewClassifier,
  compareClassifier,
  debugState,
}

export default gptAutoAPI
