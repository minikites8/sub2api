/**
 * Invoice API endpoints.
 *
 * Invoicing is a manual flow: the user submits a request with their billing
 * details and the orders it should cover, an operator issues the invoice in the
 * tax system, then records the resulting number here.
 */

import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

export type InvoiceStatus = 'pending' | 'issued' | 'rejected' | 'cancelled'
export type InvoiceEntityType = 'company' | 'individual'

/** One order covered by a request. Amounts are snapshotted at submit time. */
export interface InvoiceItem {
  id: number
  order_id: number
  description: string
  amount: number
  order_created_at?: string
}

export interface Invoice {
  id: number
  user_id: number
  /** Platform request number (INV-2026-XXXXXX), not the tax invoice number. */
  invoice_no: string
  entity_type: InvoiceEntityType
  title: string
  tax_id?: string
  delivery_email: string
  notes?: string
  amount: number
  status: InvoiceStatus
  /** The real invoice number, filled in once an operator has issued it. */
  issued_invoice_no?: string
  issued_file_url?: string
  reject_reason?: string
  reviewed_at?: string
  created_at: string
  updated_at: string
  items?: InvoiceItem[]
}

/** A paid order that is not covered by any live invoice request yet. */
export interface InvoiceableOrder {
  order_id: number
  out_trade_no: string
  description: string
  amount: number
  created_at: string
}

export interface CreateInvoicePayload {
  entity_type: InvoiceEntityType
  title: string
  tax_id?: string
  delivery_email: string
  notes?: string
  order_ids: number[]
}

export interface ListInvoiceParams {
  page?: number
  page_size?: number
  status?: string
  search?: string
}

/** Orders the user can still invoice. */
export async function getInvoiceableOrders(): Promise<InvoiceableOrder[]> {
  const { data } = await apiClient.get<{ orders: InvoiceableOrder[] }>('/invoices/invoiceable-orders')
  return data.orders ?? []
}

export async function listInvoices(params?: ListInvoiceParams): Promise<BasePaginationResponse<Invoice>> {
  const { data } = await apiClient.get<BasePaginationResponse<Invoice>>('/invoices', { params })
  return data
}

export async function getInvoice(id: number): Promise<Invoice> {
  const { data } = await apiClient.get<Invoice>(`/invoices/${id}`)
  return data
}

export async function createInvoice(payload: CreateInvoicePayload): Promise<Invoice> {
  const { data } = await apiClient.post<Invoice>('/invoices', payload)
  return data
}

/** Withdraw a pending request, which frees its orders for a new one. */
export async function cancelInvoice(id: number): Promise<void> {
  await apiClient.post(`/invoices/${id}/cancel`)
}

export default {
  getInvoiceableOrders,
  listInvoices,
  getInvoice,
  createInvoice,
  cancelInvoice,
}
