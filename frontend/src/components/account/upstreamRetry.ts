export const UPSTREAM_RETRY_ACCOUNT_KEY = 'openai_upstream_retry_account'
export const UPSTREAM_MAX_RETRIES_KEY = 'openai_upstream_max_retries'
export const MAX_UPSTREAM_RETRIES = 2147483647

export type UpstreamMaxRetries = number | ''

export function isValidUpstreamMaxRetries(value: unknown): value is UpstreamMaxRetries {
  return value === '' || (
    typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= MAX_UPSTREAM_RETRIES
  )
}

export function readUpstreamRetrySettings(credentials: Record<string, unknown> | null | undefined) {
  const limit = credentials?.[UPSTREAM_MAX_RETRIES_KEY]
  return {
    enabled: credentials?.[UPSTREAM_RETRY_ACCOUNT_KEY] === true,
    maxRetries: isValidUpstreamMaxRetries(limit) ? limit : '' as UpstreamMaxRetries
  }
}

export function applyUpstreamRetrySettings(
  credentials: Record<string, unknown>,
  enabled: boolean,
  maxRetries: UpstreamMaxRetries,
  mode: 'create' | 'edit'
): void {
  if (!enabled && mode === 'create') return
  credentials[UPSTREAM_RETRY_ACCOUNT_KEY] = enabled
  // null clears an old limit in both full saves and JSONB merge patches.
  credentials[UPSTREAM_MAX_RETRIES_KEY] = enabled && maxRetries !== '' ? maxRetries : null
}
