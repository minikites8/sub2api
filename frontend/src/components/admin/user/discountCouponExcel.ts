import type { DiscountCouponImportRow, DiscountCouponType } from '@/api/admin/users'
import type { WorkBook, WorkSheet } from 'xlsx'
import { saveAs } from 'file-saver'

export const MAX_COUPON_IMPORT_ROWS = 500
const MAX_FILE_SIZE = 5 * 1024 * 1024
const fields = ['user_id', 'email', 'coupon_type', 'min_amount', 'discount_rate', 'total_uses', 'notes'] as const
type Field = typeof fields[number]
type Translate = (key: string) => string

const aliases: Record<Field, string[]> = {
  user_id: ['user_id', 'user id', '用户id'],
  email: ['email', 'user email', '邮箱', '用户邮箱'],
  coupon_type: ['coupon_type', 'coupon type', '券类型', '折扣券类型'],
  min_amount: ['min_amount', 'minimum amount', '门槛金额', '最低金额'],
  discount_rate: ['discount_rate', 'discount rate', '折扣', '折扣（折）', '折扣(折)'],
  total_uses: ['total_uses', 'uses', 'total uses', '次数', '可用次数'],
  notes: ['notes', '备注'],
}

export class CouponExcelError extends Error {
  constructor(public code: string) { super(code) }
}

export interface ParsedCouponRow {
  data: DiscountCouponImportRow
  errorCode?: string
}

const normalize = (value: unknown) => String(value ?? '').trim().toLowerCase().replace(/\s+/g, '')
const text = (value: unknown) => String(value ?? '').trim()
const numeric = (value: unknown) => typeof value === 'number' ? value
  : /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$/.test(text(value)) ? Number(value) : NaN

export function parseCouponWorkbook(workbook: WorkBook): ParsedCouponRow[] {
  const sheet = workbook.Sheets[workbook.SheetNames[0]]
  if (!sheet?.['!ref']) throw new CouponExcelError('emptyFile')
  // Reading is bounded to 502 rows; fullref retains the original range.
  const ref = sheet['!fullref'] || sheet['!ref']
  const endRow = /\d+$/.exec(ref)?.[0]
  if (Number(endRow) > MAX_COUPON_IMPORT_ROWS + 1) throw new CouponExcelError('tooManyRows')
  const columns = new Map<Field, number>()
  // Templates start on A1 and contain at most seven meaningful columns.
  for (let col = 0; col < 32; col++) {
    const label = normalize(cell(sheet, 0, col)?.v)
    if (!label) continue
    const field = fields.find(key => aliases[key].some(alias => normalize(alias) === label))
    if (!field) continue
    if (columns.has(field)) throw new CouponExcelError('invalidHeaders')
    columns.set(field, col)
  }
  if ((!columns.has('user_id') && !columns.has('email'))
    || ['coupon_type', 'min_amount', 'discount_rate', 'total_uses'].some(key => !columns.has(key as Field))) {
    throw new CouponExcelError('invalidHeaders')
  }
  const rows: ParsedCouponRow[] = []
  for (let row = 1; row < Number(endRow); row++) {
    const get = (field: Field) => cell(sheet, row, columns.get(field) ?? -1)
    if (fields.every(field => !text(get(field)?.v) && !get(field)?.f)) continue
    const type = normalize(get('coupon_type')?.v)
    const couponType = ['subscription', '订阅', '订阅券', '订阅优惠券'].includes(type) ? 'subscription'
      : ['recharge', '充值', '充值券', '充值折扣券', '充值优惠券'].includes(type) ? 'recharge' : ''
    const rawID = text(get('user_id')?.v)
    const data: DiscountCouponImportRow = {
      row_number: row + 1,
      user_id: rawID ? numeric(rawID) : undefined,
      email: text(get('email')?.v) || undefined,
      coupon_type: couponType as DiscountCouponType,
      min_amount: numeric(get('min_amount')?.v),
      discount_rate: numeric(get('discount_rate')?.v),
      total_uses: numeric(get('total_uses')?.v),
      notes: text(get('notes')?.v),
    }
    let errorCode: string | undefined
    if (fields.some(field => get(field)?.f)) errorCode = 'formulaCell'
    else if ((rawID && (!Number.isSafeInteger(data.user_id) || data.user_id! <= 0)) || (!rawID && !data.email)) errorCode = 'INVALID_COUPON_USER'
    else if (data.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(data.email)) errorCode = 'INVALID_COUPON_EMAIL'
    else if (!couponType) errorCode = 'INVALID_COUPON_TYPE'
    else if (!Number.isFinite(data.min_amount) || data.min_amount < 0.01 || data.min_amount >= 1e12) errorCode = 'INVALID_COUPON_MIN_AMOUNT'
    else if (!Number.isFinite(data.discount_rate) || data.discount_rate < 0.01 || data.discount_rate > 9.99) errorCode = 'INVALID_COUPON_DISCOUNT'
    else if (!Number.isInteger(data.total_uses) || data.total_uses <= 0 || data.total_uses > 2147483647) errorCode = 'INVALID_COUPON_USES'
    else if (Array.from(data.notes || '').length > 2000) errorCode = 'INVALID_COUPON_NOTES'
    rows.push({ data, errorCode })
  }
  if (rows.length === 0) throw new CouponExcelError('emptyFile')
  return rows
}

function cell(sheet: WorkSheet, row: number, col: number) {
  if (col < 0) return undefined
  // This importer supports the template's first 32 columns (A through AF).
  const name = col < 26 ? String.fromCharCode(65 + col) : `A${String.fromCharCode(65 + col - 26)}`
  return sheet[`${name}${row + 1}`]
}

export async function readCouponExcel(file: File): Promise<ParsedCouponRow[]> {
  if (!/\.(xlsx|xls)$/i.test(file.name)) throw new CouponExcelError('invalidFile')
  if (file.size > MAX_FILE_SIZE) throw new CouponExcelError('fileTooLarge')
  const XLSX = await import('xlsx')
  const buffer = await file.arrayBuffer()
  let workbook: WorkBook
  try {
    workbook = XLSX.read(buffer, { type: 'array', sheetRows: MAX_COUPON_IMPORT_ROWS + 2, cellFormula: true, sheets: 0 })
  } catch {
    throw new CouponExcelError('invalidFile')
  }
  return parseCouponWorkbook(workbook)
}

export async function downloadCouponTemplate(type: DiscountCouponType, t: Translate, user?: { id: number; email: string }) {
  const XLSX = await import('xlsx')
  const workbook = XLSX.utils.book_new()
  const headers = fields.map(field => t(`admin.users.couponImport.columns.${field}`))
  const data = XLSX.utils.aoa_to_sheet([headers])
  data['!cols'] = [{ wch: 14 }, { wch: 30 }, { wch: 20 }, { wch: 18 }, { wch: 18 }, { wch: 16 }, { wch: 40 }]
  const instructions = XLSX.utils.aoa_to_sheet([
    [t('admin.users.couponImport.templateInstructions')],
    [t('admin.users.couponImport.templateIdentity')],
    [t('admin.users.couponImport.templateExample')],
    headers,
    [user?.id || '', user?.email || 'user@example.com', type, type === 'subscription' ? 20 : 100, 8, 3, ''],
    [t('admin.users.couponImport.templateHint')],
    [t('admin.users.couponImport.fileHint')],
  ])
  instructions['!cols'] = data['!cols']
  instructions['!merges'] = [0, 1, 2, 5, 6].map(row => ({ s: { r: row, c: 0 }, e: { r: row, c: 6 } }))
  instructions['!rows'] = Array.from({ length: 7 }, () => ({ hpt: 24 }))
  XLSX.utils.book_append_sheet(workbook, data, t('admin.users.couponImport.dataSheet'))
  XLSX.utils.book_append_sheet(workbook, instructions, t('admin.users.couponImport.instructionsSheet'))
  saveAs(new Blob([XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  }), `${type}-discount-coupons-template.xlsx`)
}
