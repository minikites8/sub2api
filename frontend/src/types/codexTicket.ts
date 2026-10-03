export type CodexTicketState = 'ready' | 'harvesting' | 'waiting' | 'cooldown' | 'paused' | 'token_invalid' | 'proxy_missing' | 'disabled' | 'expired'

export interface CodexTicketStatus {
  model: string
  state?: CodexTicketState
  ready: boolean
  remaining_seconds: number
  expires_at?: string
  length?: number
  target_length?: number
  attempts?: number
  harvesting?: boolean
  harvest_enabled?: boolean
  next_harvest_at?: string
  probe?: { result: string; http_status?: number; checked_at: string; next_probe_at?: string }
}

export interface CodexTicketLogEntry {
  id: number
  time: string
  attempt: number
  event: 'started' | 'acquired' | 'miss' | 'error'
  reason: string
  http_status?: number
  ticket_length?: number
  target_length: number
  duration_ms?: number
  gateway?: string
  egress_ip?: string
  egress_country_code?: string
  egress_error?: { reason: string; http_status?: number }
}

export interface CodexTicketLogsResponse {
  gateway_blacklist?: string[]
  model: string
  entries: CodexTicketLogEntry[]
  status: CodexTicketStatus | null
  limit: number
  fetched_at: string
}
