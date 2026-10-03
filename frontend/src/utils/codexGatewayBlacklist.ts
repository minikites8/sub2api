export const CODEX_GATEWAY_BLACKLIST_EXTRA_KEY = 'openai_codex_ticket_gateway_blacklist'

export function normalizeCodexGatewayName(value: string): string {
  const gateway = value.trim().toLowerCase()
  const alias = gateway.match(/^(?:unified[-_.]?)?(\d{1,5})$/)
  const host = gateway.match(/^(?:chat\.gateway\.)?(unified-\d{1,5})(?:\.api\.openai\.com)?$/)
  return alias ? `unified-${alias[1]}` : host?.[1] ?? gateway
}

export function parseCodexGatewayBlacklist(value: unknown): string[] {
  const values = typeof value === 'string' ? value.split(/[,\r\n]/) : value == null ? [] : value
  if (!Array.isArray(values) || values.some(item => typeof item !== 'string')) {
    throw new Error('Invalid gateway blacklist')
  }
  const gateways = values.map(normalizeCodexGatewayName).filter(Boolean)
  if (gateways.some(gateway => !/^unified-\d{1,5}$/.test(gateway))) {
    throw new Error('Invalid gateway blacklist')
  }
  return [...new Set(gateways)]
}

export function readCodexGatewayBlacklist(extra?: Record<string, unknown>): string[] {
  try {
    return parseCodexGatewayBlacklist(extra?.[CODEX_GATEWAY_BLACKLIST_EXTRA_KEY])
  } catch {
    return []
  }
}
