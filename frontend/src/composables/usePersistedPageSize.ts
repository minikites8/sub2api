import {
  DEFAULT_TABLE_PAGE_SIZE,
  getConfiguredTableDefaultPageSize,
  normalizeTablePageSize
} from '@/utils/tablePreferences'

const STORAGE_KEY = 'table-page-size'

export function getPersistedPageSize(fallback = getConfiguredTableDefaultPageSize()): number {
  const configuredDefault = getConfiguredTableDefaultPageSize()
  if (
    configuredDefault !== DEFAULT_TABLE_PAGE_SIZE ||
    (typeof window !== 'undefined' && window.__APP_CONFIG__?.table_default_page_size !== undefined)
  ) {
    return normalizeTablePageSize(configuredDefault)
  }

  if (typeof window !== 'undefined') {
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY)
      if (stored !== null) {
        const parsed = Number(stored)
        if (Number.isFinite(parsed)) {
          return normalizeTablePageSize(parsed)
        }
      }
    } catch (error) {
      console.warn('Failed to read persisted page size:', error)
    }
  }
  return normalizeTablePageSize(getConfiguredTableDefaultPageSize() || fallback)
}

export function setPersistedPageSize(size: number): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(size))
  } catch (error) {
    console.warn('Failed to persist page size:', error)
  }
}
