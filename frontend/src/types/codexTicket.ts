export interface CodexTicketStatus {
  model: string
  state: 'ready' | 'harvesting' | 'waiting' | 'cooldown' | 'paused' | 'token_invalid' | 'proxy_missing' | 'disabled'
  ready: boolean
  remaining_seconds: number
  expires_at?: string
  length?: number
  target_length: number
  attempts: number
  harvesting: boolean
  harvest_enabled: boolean
  next_harvest_at?: string
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
  egress_ip?: string
  egress_country_code?: string
  egress_error?: { reason: string; http_status?: number }
}

export interface CodexTicketLogsResponse {
  model: string
  entries: CodexTicketLogEntry[]
  status: CodexTicketStatus | null
  limit: number
  fetched_at: string
}
