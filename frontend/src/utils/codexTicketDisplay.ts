import type { CodexTicketStatus } from '@/types/codexTicket'

export function codexTicketModelLabel(model: string): string {
  return /^gpt-[\d.]+-(astra|sol)$/.exec(model)?.[1] ?? model
}
export function codexTicketRemaining(status: CodexTicketStatus, receivedAt: number, now: number): number {
  return Math.max(0, Math.floor(status.remaining_seconds - Math.max(0, now - receivedAt) / 1000))
}
export function codexTicketDuration(seconds: number): string {
  const value = Math.max(0, Math.floor(seconds))
  return `${Math.floor(value / 60)}m${String(value % 60).padStart(2, '0')}s`
}
export function codexTicketState(status: CodexTicketStatus, receivedAt: number, now: number): string {
  if (status.state === 'ready' && codexTicketRemaining(status, receivedAt, now) === 0) return 'expired'
  return status.state
}
export function codexTicketStateClass(state: string): string {
  if (state === 'ready') return 'text-emerald-600 dark:text-emerald-400'
  if (state === 'harvesting') return 'text-blue-600 dark:text-blue-400'
  if (state === 'token_invalid') return 'text-red-600 dark:text-red-400'
  if (['expired', 'cooldown', 'proxy_missing'].includes(state)) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-500 dark:text-gray-400'
}
