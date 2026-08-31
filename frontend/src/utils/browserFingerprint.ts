import FingerprintJS from '@fingerprintjs/fingerprintjs'

const STORAGE_KEY = 'sub2api.browser_fingerprints'
type FingerprintAgent = Awaited<ReturnType<typeof FingerprintJS.load>>
let fingerprintPromise: Promise<string[]> | undefined
let fingerprintJSAgent: Promise<FingerprintAgent | null> | undefined

function loadFingerprintJS(): Promise<FingerprintAgent | null> {
  fingerprintJSAgent ||= FingerprintJS.load().catch(() => null)
  return fingerprintJSAgent
}

function collectFingerprintMaterial(): string {
  const nav = typeof navigator === 'undefined' ? undefined : navigator
  const deviceMemory = nav ? (nav as Navigator & { deviceMemory?: number }).deviceMemory : undefined
  const screenInfo = typeof screen === 'undefined' ? undefined : screen
  let timezone = ''
  try { timezone = Intl.DateTimeFormat().resolvedOptions().timeZone } catch { timezone = '' }
	return [
		['ua', nav?.userAgent], ['lang', nav?.language], ['platform', nav?.platform],
		['cpu', nav?.hardwareConcurrency], ['memory', deviceMemory], ['touch', nav?.maxTouchPoints],
		['screen', `${screenInfo?.width ?? ''}x${screenInfo?.height ?? ''}`], ['depth', screenInfo?.colorDepth], ['timezone', timezone]
	].map(([key, value]) => `${key}=${String(value ?? '')}`).join(';')
}

export async function getBrowserFingerprint(): Promise<string> {
	try {
		const agent = await loadFingerprintJS()
		if (!agent) return ''
		const result = await agent.get()
		return `fingerprintjs-v4:${result.visitorId}`
	} catch {
		return ''
	}
}

async function collectBrowserFingerprints(): Promise<string[]> {
  const [fingerprintJSValue, deviceValue] = await Promise.all([
    getBrowserFingerprint(),
    Promise.resolve(collectFingerprintMaterial())
  ])
  let previous: string[] = []
  try { previous = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]'); if (!Array.isArray(previous)) previous = [] } catch { previous = [] }
  const values = Array.from(new Set([fingerprintJSValue, deviceValue, ...previous].filter((value): value is string => typeof value === 'string' && value.length > 0))).slice(0, 8)
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(values)) } catch { /* private browsing can disable storage */ }
  return values
}

export function getBrowserFingerprints(): Promise<string[]> {
  fingerprintPromise ||= collectBrowserFingerprints()
  return fingerprintPromise
}
