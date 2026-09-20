import { apiClient } from '../client'
import type { CodexTicketLogsResponse, CodexTicketStatus } from '@/types/codexTicket'

export async function getCodexTicketLogs(accountId: number, model: string, signal?: AbortSignal): Promise<CodexTicketLogsResponse> {
  const { data } = await apiClient.get<CodexTicketLogsResponse>(`/admin/accounts/${accountId}/codex-ticket-logs`, {
    params: { model }, signal, timeout: 10000
  })
  return data
}

export async function getBatchCodexTickets(accountIds: number[], signal?: AbortSignal): Promise<Record<string, CodexTicketStatus[]>> {
  const { data } = await apiClient.post<{ statuses: Record<string, CodexTicketStatus[]> }>(
    '/admin/accounts/codex-tickets/batch', { account_ids: accountIds }, { signal, timeout: 10000 }
  )
  return data.statuses
}
