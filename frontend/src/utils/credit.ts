export const CREDITS_PER_RMB = 100
export const USD_QUOTA_PER_CREDIT = 0.01
export const CREDITS_PER_USD = 1 / USD_QUOTA_PER_CREDIT

export const usdToCredits = (usd: number | null | undefined): number =>
  Number(usd || 0) * CREDITS_PER_USD

export const formatCredits = (credits: number, maximumFractionDigits = 2): string =>
  new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits,
  }).format(credits)
