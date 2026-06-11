export function getReverseAvailableQuota(balance: number, rate: number | null): number | null {
  if (rate === null || Number.isNaN(rate)) return null
  if (rate === 0) return Number.POSITIVE_INFINITY
  return balance / rate
}
