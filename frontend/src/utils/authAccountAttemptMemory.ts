const MAX_AUTH_ACCOUNT_ATTEMPTS = 8
const STORAGE_KEY = 'sub2api.auth_account_attempts'

export const AUTH_ACCOUNT_ATTEMPTS_HEADER = 'X-Browser-Account-Attempts'

const accountAttempts = new Set<string>()

function normalizeAccountIdentifier(value: string): string {
  const normalized = value.trim().toLowerCase()
  if (normalized.length > 320 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(normalized)) return ''
  return normalized
}

function syncFromStorage(): void {
  if (typeof localStorage === 'undefined') return
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]')
    if (!Array.isArray(stored)) return
    for (const account of stored.slice(-MAX_AUTH_ACCOUNT_ATTEMPTS)) {
      const normalized = normalizeAccountIdentifier(String(account ?? ''))
      if (normalized) accountAttempts.add(normalized)
    }
    while (accountAttempts.size > MAX_AUTH_ACCOUNT_ATTEMPTS) {
      const oldest = accountAttempts.values().next().value
      if (typeof oldest === 'string') accountAttempts.delete(oldest)
    }
  } catch {
    // Storage access can fail in hardened private-browsing environments.
  }
}

function persistToStorage(): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(Array.from(accountAttempts)))
  } catch {
    // In-memory tracking remains available when persistent storage is blocked.
  }
}

export function getAuthAccountAttempts(): string[] {
  syncFromStorage()
  return Array.from(accountAttempts)
}

export function rememberAuthAccountAttempt(account: string): string[] {
  syncFromStorage()
  const normalized = normalizeAccountIdentifier(account)
  if (!normalized) return getAuthAccountAttempts()

  if (!accountAttempts.has(normalized) && accountAttempts.size >= MAX_AUTH_ACCOUNT_ATTEMPTS) {
    const oldest = accountAttempts.values().next().value
    if (typeof oldest === 'string') accountAttempts.delete(oldest)
  }
  accountAttempts.add(normalized)
  persistToStorage()
  return getAuthAccountAttempts()
}

export function clearAuthAccountAttemptMemory(): void {
  accountAttempts.clear()
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Keep test/reset behavior deterministic when storage is unavailable.
  }
}
