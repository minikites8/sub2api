import { describe, expect, it, vi } from 'vitest'
import * as XLSX from 'xlsx'
import { downloadCouponTemplate, parseCouponWorkbook, readCouponExcel } from '../discountCouponExcel'

const { saveAs } = vi.hoisted(() => ({ saveAs: vi.fn() }))
vi.mock('file-saver', () => ({ saveAs }))

const headers = ['user_id', 'email', 'coupon_type', 'min_amount', 'discount_rate', 'total_uses', 'notes']
function workbook(rows: unknown[][]) {
  const book = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(book, XLSX.utils.aoa_to_sheet(rows), 'Coupons')
  return book
}

describe('discount coupon Excel', () => {
  it('parses mixed coupon types and preserves Excel row numbers across blank rows', () => {
    const rows = parseCouponWorkbook(workbook([
      headers,
      [42, '', 'subscription', 20, 8.5, 3, ' retention '],
      [],
      ['', ' user@example.com ', 'recharge', 100, 8, 2, ''],
    ]))
    expect(rows).toEqual([
      { data: { row_number: 2, user_id: 42, email: undefined, coupon_type: 'subscription', min_amount: 20, discount_rate: 8.5, total_uses: 3, notes: 'retention' }, errorCode: undefined },
      { data: { row_number: 4, user_id: undefined, email: 'user@example.com', coupon_type: 'recharge', min_amount: 100, discount_rate: 8, total_uses: 2, notes: '' }, errorCode: undefined },
    ])
  })

  it('accepts localized template headers and Chinese coupon type names', () => {
    const rows = parseCouponWorkbook(workbook([
      ['用户ID', '用户邮箱', '券类型', '门槛金额', '折扣（折）', '可用次数', '备注'],
      ['42', '', '订阅优惠券', '20.00', '8', '3', ''],
    ]))
    expect(rows[0].errorCode).toBeUndefined()
    expect(rows[0].data.coupon_type).toBe('subscription')
  })

  it.each([
    [[9007199254740992, '', 'recharge', 100, 8, 1], 'INVALID_COUPON_USER'],
    [[42, 'invalid', 'recharge', 100, 8, 1], 'INVALID_COUPON_EMAIL'],
    [[42, '', 'promo', 100, 8, 1], 'INVALID_COUPON_TYPE'],
    [[42, '', 'recharge', 0, 8, 1], 'INVALID_COUPON_MIN_AMOUNT'],
    [[42, '', 'recharge', 100, '80%', 1], 'INVALID_COUPON_DISCOUNT'],
    [[42, '', 'recharge', 100, 10, 1], 'INVALID_COUPON_DISCOUNT'],
    [[42, '', 'recharge', 100, 8, 1.5], 'INVALID_COUPON_USES'],
  ])('reports invalid row values: %s', (row, code) => {
    expect(parseCouponWorkbook(workbook([headers, row as unknown[]]))[0].errorCode).toBe(code)
  })

  it('rejects formula cells even when they have cached numeric values', () => {
    const book = workbook([headers, [42, '', 'recharge', 100, 8, 1]])
    book.Sheets.Coupons.D2.f = '50*2'
    expect(parseCouponWorkbook(book)[0].errorCode).toBe('formulaCell')
  })

  it('rejects incomplete headers, empty templates, and batches over 500 rows', () => {
    expect(() => parseCouponWorkbook(workbook([headers.slice(0, 4), [42, '', 'recharge', 100]]))).toThrow('invalidHeaders')
    expect(() => parseCouponWorkbook(workbook([headers]))).toThrow('emptyFile')
    expect(() => parseCouponWorkbook(workbook([headers, ...Array.from({ length: 501 }, () => [42, '', 'recharge', 100, 8, 1])]))).toThrow('tooManyRows')
    const duplicateHeaders = workbook([[...headers, 'User ID'], [42, '', 'recharge', 100, 8, 1]])
    expect(() => parseCouponWorkbook(duplicateHeaders)).toThrow('invalidHeaders')
  })

  it('reads saved XLSX bytes and enforces file limits', async () => {
    for (const [bookType, extension] of [['xlsx', 'xlsx'], ['biff8', 'xls']] as const) {
      const buffer = XLSX.write(workbook([headers, [42, '', 'recharge', 100, 8, 1]]), { type: 'array', bookType })
      const file = { name: `coupons.${extension}`, size: buffer.byteLength, arrayBuffer: async () => buffer } as File
      expect((await readCouponExcel(file))[0].data.min_amount).toBe(100)
      await expect(readCouponExcel({ ...file, name: 'coupons.csv' } as File)).rejects.toThrow('invalidFile')
      await expect(readCouponExcel({ ...file, size: 6 * 1024 * 1024 } as File)).rejects.toThrow('fileTooLarge')
    }
  })

  it('downloads real XLSX templates with empty data and a separate typed example', async () => {
    const t = (key: string) => key.split('.').pop()!
    for (const type of ['recharge', 'subscription'] as const) {
      await downloadCouponTemplate(type, t)
      const [blob, filename] = saveAs.mock.calls.at(-1)!
      const bytes = await new Promise<ArrayBuffer>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve(reader.result as ArrayBuffer)
        reader.onerror = reject
        reader.readAsArrayBuffer(blob)
      })
      const book = XLSX.read(bytes, { type: 'array' })
      const data = XLSX.utils.sheet_to_json(book.Sheets[book.SheetNames[0]], { header: 1 })
      const example = XLSX.utils.sheet_to_json<unknown[]>(book.Sheets[book.SheetNames[1]], { header: 1 })[4]
      expect(data).toEqual([headers])
      expect(example[2]).toBe(type)
      expect(example[3]).toBe(type === 'subscription' ? 20 : 100)
      expect(example[4]).toBe(8)
      expect(filename).toBe(`${type}-discount-coupons-template.xlsx`)
    }
  })
})
