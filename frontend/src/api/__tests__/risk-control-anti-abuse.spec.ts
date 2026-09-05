import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { listAntiAbuseEvents } from '@/api/admin/riskControl'

describe('risk-control anti-abuse event refresh', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({
      data: { items: [], total: 0, page: 1, page_size: 20, pages: 1 },
    })
  })

  it('bypasses browser and proxy caches for every event reload', async () => {
    const now = vi.spyOn(Date, 'now')
      .mockReturnValueOnce(1001)
      .mockReturnValueOnce(1002)

    await listAntiAbuseEvents({ page: 1, page_size: 20 })
    await listAntiAbuseEvents({ page: 1, page_size: 20 })

    expect(get).toHaveBeenNthCalledWith(1, '/admin/risk-control/anti-abuse/events', {
      params: { page: 1, page_size: 20, _refresh: 1001 },
      headers: { 'Cache-Control': 'no-cache' },
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/risk-control/anti-abuse/events', {
      params: { page: 1, page_size: 20, _refresh: 1002 },
      headers: { 'Cache-Control': 'no-cache' },
    })

    now.mockRestore()
  })
})
