import { beforeEach, describe, expect, it, vi } from 'vitest'

describe('auth account attempt memory', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('persists normalized distinct accounts across module reloads', async () => {
    const first = await import('../authAccountAttemptMemory')
    expect(first.rememberAuthAccountAttempt(' First@Example.com ')).toEqual(['first@example.com'])
    expect(first.rememberAuthAccountAttempt('SECOND@example.com')).toEqual([
      'first@example.com',
      'second@example.com',
    ])
    expect(first.rememberAuthAccountAttempt('first@example.com')).toHaveLength(2)

    vi.resetModules()
    const reloaded = await import('../authAccountAttemptMemory')
    expect(reloaded.getAuthAccountAttempts()).toEqual(['first@example.com', 'second@example.com'])
  })

  it('keeps the latest eight valid accounts', async () => {
    const tracker = await import('../authAccountAttemptMemory')
    tracker.rememberAuthAccountAttempt('invalid')
    for (let index = 0; index < 10; index += 1) {
      tracker.rememberAuthAccountAttempt(`user${index}@example.com`)
    }

    expect(tracker.getAuthAccountAttempts()).toEqual(
      Array.from({ length: 8 }, (_, index) => `user${index + 2}@example.com`),
    )
  })

  it('merges accounts written by another browser tab before persisting', async () => {
    const tracker = await import('../authAccountAttemptMemory')
    tracker.rememberAuthAccountAttempt('first@example.com')
    localStorage.setItem(
      'sub2api.auth_account_attempts',
      JSON.stringify(['first@example.com', 'second@example.com']),
    )

    tracker.rememberAuthAccountAttempt('third@example.com')

    expect(tracker.getAuthAccountAttempts()).toEqual([
      'first@example.com',
      'second@example.com',
      'third@example.com',
    ])
  })
})
