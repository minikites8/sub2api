import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { subscribeCodexTicketStatuses } from '../useCodexTicketStatuses'

const { getBatch } = vi.hoisted(() => ({ getBatch: vi.fn() }))
vi.mock('@/api/admin/codexTickets', () => ({ getBatchCodexTickets: getBatch }))
beforeEach(() => { vi.useFakeTimers(); getBatch.mockReset().mockResolvedValue({}); vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible') })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })
describe('shared Codex ticket polling', () => {
  it('batches visible accounts, deduplicates IDs and stops after the last subscriber', async () => {
    const a = vi.fn(), b = vi.fn()
    const removeA = subscribeCodexTicketStatuses(71, a)
    const removeB = subscribeCodexTicketStatuses(72, b)
    const removeDuplicate = subscribeCodexTicketStatuses(71, vi.fn())
    await vi.advanceTimersByTimeAsync(0); await flushPromises()
    expect(getBatch).toHaveBeenCalledTimes(1); expect(getBatch.mock.calls[0][0]).toEqual([71, 72]); expect(a).toHaveBeenCalledWith([])
    removeA(); removeDuplicate(); await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(getBatch.mock.calls[1][0]).toEqual([72]); removeB()
    await vi.advanceTimersByTimeAsync(10000); expect(getBatch).toHaveBeenCalledTimes(2)
  })
  it('aborts pending polling when hidden and retries on visibility', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    getBatch.mockImplementationOnce(() => new Promise(() => {}))
    const stop = subscribeCodexTicketStatuses(71, vi.fn())
    await vi.advanceTimersByTimeAsync(0)
    const signal = getBatch.mock.calls[0][1] as AbortSignal
    visibility.mockReturnValue('hidden'); document.dispatchEvent(new Event('visibilitychange')); expect(signal.aborted).toBe(true)
    visibility.mockReturnValue('visible'); document.dispatchEvent(new Event('visibilitychange')); await vi.advanceTimersByTimeAsync(0); await flushPromises()
    expect(getBatch).toHaveBeenCalledTimes(2); stop()
  })
})
