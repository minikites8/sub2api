import type { CodexTicketState, CodexTicketStatus } from '@/types/codexTicket'

const ticketStates = new Set(['ready', 'harvesting', 'waiting', 'cooldown', 'paused', 'token_invalid', 'proxy_missing', 'disabled', 'expired'])

export function codexTicketModelLabel(model: string): string {
  return model
}
export function codexTicketRemaining(status: CodexTicketStatus, receivedAt: number, now: number): number {
  if (!Number.isFinite(status.remaining_seconds)) return 0
  return Math.max(0, Math.floor(status.remaining_seconds - Math.max(0, now - receivedAt) / 1000))
}
export function codexTicketAttempts(status: CodexTicketStatus): number {
  return typeof status.attempts === 'number' && Number.isFinite(status.attempts) ? Math.max(0, Math.floor(status.attempts)) : 0
}
export function codexTicketDuration(seconds: number): string {
  const value = Math.max(0, Math.floor(seconds))
  return `${Math.floor(value / 60)}m${String(value % 60).padStart(2, '0')}s`
}
export function codexTicketState(status: CodexTicketStatus, receivedAt = Date.now(), now = Date.now()): CodexTicketState {
  let state = status.state
  if (!state || !ticketStates.has(state)) {
    if (status.ready) state = 'ready'
    else if (status.harvesting) state = 'harvesting'
    else if (status.harvest_enabled === false) state = 'paused'
    else if (status.probe?.result === 'token_error' || [401, 403].includes(status.probe?.http_status ?? 0)) state = 'token_invalid'
    else if (Date.parse(status.probe?.next_probe_at ?? status.next_harvest_at ?? '') > now) state = 'cooldown'
    else state = 'waiting'
  }
  if (state === 'ready' && codexTicketRemaining(status, receivedAt, now) === 0) return 'expired'
  return state
}
export function codexTicketStateClass(state: string): string {
  if (state === 'ready') return 'text-emerald-600 dark:text-emerald-400'
  if (state === 'harvesting') return 'text-blue-600 dark:text-blue-400'
  if (state === 'token_invalid') return 'text-red-600 dark:text-red-400'
  if (['expired', 'cooldown', 'proxy_missing'].includes(state)) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-500 dark:text-gray-400'
}
