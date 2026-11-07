import { describe, expect, it } from 'vitest'
import { formatUnavailableTimeWindow } from '../unavailable-time'

describe('daily unavailable hours', () => {
  it('discloses the server timezone and midnight crossing', () => {
    expect(formatUnavailableTimeWindow({ unavailable_time_enabled: true, unavailable_start: '22:00', unavailable_end: '08:00' }, 'UTC+08:00', '次日')).toBe('22:00–次日 08:00 (UTC+08:00)')
    expect(formatUnavailableTimeWindow({ unavailable_time_enabled: true, unavailable_start: '08:00', unavailable_end: '22:00' }, 'UTC+08:00')).toBe('08:00–22:00 (UTC+08:00)')
  })

  it('omits disabled or absent schedules', () => {
    expect(formatUnavailableTimeWindow(null)).toBe('')
    expect(formatUnavailableTimeWindow({ unavailable_time_enabled: false, unavailable_start: '08:00', unavailable_end: '22:00' })).toBe('')
    expect(formatUnavailableTimeWindow({ unavailable_time_enabled: true })).toBe('')
  })
})
