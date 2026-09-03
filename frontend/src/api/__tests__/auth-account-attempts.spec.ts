import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post, getBrowserFingerprints } = vi.hoisted(() => ({
  post: vi.fn(),
  getBrowserFingerprints: vi.fn(),
}))

vi.mock('../client', () => ({
  apiClient: { post },
}))

vi.mock('@/utils/browserFingerprint', () => ({
  getBrowserFingerprints,
}))

import { login, register } from '../auth'
import { clearAuthAccountAttemptMemory } from '@/utils/authAccountAttemptMemory'

const authResponse = {
  access_token: 'access-token',
  refresh_token: 'refresh-token',
  expires_in: 3600,
  token_type: 'Bearer',
  user: { id: 1, email: 'second@example.com' },
}

describe('authentication account attempt tracking', () => {
  beforeEach(() => {
    post.mockReset()
    getBrowserFingerprints.mockReset()
    localStorage.clear()
    clearAuthAccountAttemptMemory()
  })

  it('retains failed and successful login accounts in later requests', async () => {
    post.mockRejectedValueOnce(new Error('invalid credentials'))
    await expect(login({ email: 'First@Example.com', password: 'wrong' })).rejects.toThrow('invalid credentials')

    post.mockResolvedValueOnce({ data: authResponse })
    await login({ email: 'second@example.com', password: 'correct' })

    expect(post.mock.calls[1][1]).toMatchObject({
      account_attempts: ['first@example.com', 'second@example.com'],
    })
  })

  it('includes persisted login accounts in registration requests', async () => {
    post.mockResolvedValue({ data: authResponse })
    getBrowserFingerprints.mockResolvedValue(['fingerprintjs-v4:test'])

    await login({ email: 'login@example.com', password: 'correct' })
    await register({ email: 'register@example.com', password: 'secret123' })

    expect(post.mock.calls[1][1]).toMatchObject({
      account_attempts: ['login@example.com', 'register@example.com'],
      browser_fingerprints: ['fingerprintjs-v4:test'],
    })
  })
})
