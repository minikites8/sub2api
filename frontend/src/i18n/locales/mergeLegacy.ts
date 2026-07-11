type LocaleRecord = Record<string, unknown>

function isLocaleRecord(value: unknown): value is LocaleRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function mergeMissingLocaleKeys<T extends LocaleRecord>(current: T, legacy: LocaleRecord): T {
  const target = current as LocaleRecord
  for (const [key, legacyValue] of Object.entries(legacy)) {
    if (!(key in target)) {
      target[key] = legacyValue
      continue
    }
    if (isLocaleRecord(target[key]) && isLocaleRecord(legacyValue)) {
      mergeMissingLocaleKeys(target[key], legacyValue)
    }
  }
  return current
}
