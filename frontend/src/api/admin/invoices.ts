/**
 * Admin invoice API endpoints.
 *
 * The operator side of the manual invoicing flow: read a request, issue the
 * invoice in the tax system, then record the number here — or reject it with a
 * reason so the user knows what to fix.
 */

import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { Invoice } from '@/api/invoice'

export type { Invoice } from '@/api/invoice'

export interface ListInvoicesParams {
  page?: number
  page_size?: number
  status?: string
  search?: string
}

export interface IssueInvoicePayload {
  issued_invoice_no: string
  issued_file_url?: string
}

const BASE = '/admin/invoices'

export async function listInvoices(params?: ListInvoicesParams): Promise<BasePaginationResponse<Invoice>> {
  const { data } = await apiClient.get<BasePaginationResponse<Invoice>>(BASE, { params })
  return data
}

/** Record the real invoice number after issuing it offline. */
export async function issueInvoice(id: number, payload: IssueInvoicePayload): Promise<Invoice> {
  const { data } = await apiClient.post<Invoice>(`${BASE}/${id}/issue`, payload)
  return data
}

/** Reject a request; this frees its orders so the user can resubmit. */
export async function rejectInvoice(id: number, reason: string): Promise<Invoice> {
  const { data } = await apiClient.post<Invoice>(`${BASE}/${id}/reject`, { reason })
  return data
}

export default {
  listInvoices,
  issueInvoice,
  rejectInvoice,
}
