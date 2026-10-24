import { apiClient } from './client'

// Customer-facing GPT Auto routing history. The backend scopes every call to
// the signed-in user's own live gpt-auto requests; no owner id is ever sent.

export type GPTAutoCustomerExportFormat = 'csv' | 'jsonl'

export interface GPTAutoCustomerStatus {
  /** Whether the user has any gpt-auto request in the last 93 days. */
  has_history: boolean
  virtual_model: string
  export_enabled: boolean
  export_schema_version: string
}

export interface GPTAutoCustomerLogEntry {
  created_at: string
  request_id: string
  api_key_id?: number
  endpoint: string
  requested_model: string
  executed_model: string
  reasoning_effort: string
  status: string
  total_latency_ms?: number
  fallback: boolean
  routing_explanation: string
}

export interface GPTAutoCustomerLogPage {
  items: GPTAutoCustomerLogEntry[]
  total: number
  page: number
  page_size: number
}

export interface GPTAutoCustomerRange {
  start_time: string
  end_time: string
}

export interface GPTAutoCustomerExportSummary {
  rows: number
  truncated: boolean
  max_rows: number
}

const base = '/gpt-auto'

export async function getStatus(): Promise<GPTAutoCustomerStatus> {
  const { data } = await apiClient.get<GPTAutoCustomerStatus>(`${base}/status`)
  return data
}

export async function listRoutingLogs(params: GPTAutoCustomerRange & { page: number; page_size: number }): Promise<GPTAutoCustomerLogPage> {
  const { data } = await apiClient.get<GPTAutoCustomerLogPage>(`${base}/routing-logs`, { params })
  return data
}

export async function countRoutingLogExport(params: GPTAutoCustomerRange): Promise<GPTAutoCustomerExportSummary> {
  const { data } = await apiClient.get<GPTAutoCustomerExportSummary>(`${base}/routing-logs/export/count`, { params })
  return data
}

export async function exportRoutingLogs(params: GPTAutoCustomerRange & { format: GPTAutoCustomerExportFormat }): Promise<{ blob: Blob; filename: string }> {
  const response = await apiClient.get(`${base}/routing-logs/export`, { params, responseType: 'blob', timeout: 11 * 60 * 1000 })
  const match = /filename="([^"]+)"/.exec(String(response.headers?.['content-disposition'] ?? ''))
  return { blob: response.data as Blob, filename: match ? match[1] : `gpt-auto-routing-customer.${params.format}` }
}

export const gptAutoUserAPI = { getStatus, listRoutingLogs, countRoutingLogExport, exportRoutingLogs }

export default gptAutoUserAPI
