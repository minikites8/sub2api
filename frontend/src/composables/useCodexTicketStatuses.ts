import { getBatchCodexTickets } from '@/api/admin/codexTickets'
import type { CodexTicketStatus } from '@/types/codexTicket'

type Listener = (statuses: CodexTicketStatus[]) => void
const subscribers = new Map<number, Set<Listener>>()
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let polling = false
let generation = 0
const isVisible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden'

function schedule(delay = 5000) {
  if (timer) clearTimeout(timer)
  timer = undefined
  if (subscribers.size && isVisible()) timer = setTimeout(poll, delay)
}
async function poll() {
  timer = undefined
  if (polling || !subscribers.size || !isVisible()) return
  polling = true
  const run = generation
  const pending = new AbortController()
  controller = pending
  const ids = [...subscribers.keys()]
  try {
    for (let offset = 0; offset < ids.length; offset += 100) {
      const batch = ids.slice(offset, offset + 100)
      const statuses = await getBatchCodexTickets(batch, pending.signal)
      if (run !== generation || pending.signal.aborted) return
      for (const id of batch) subscribers.get(id)?.forEach(listener => listener(statuses[String(id)] ?? []))
    }
  } catch {
    // Preserve the last snapshot; countdowns continue to reflect its age.
  } finally {
    if (run === generation) { polling = false; controller = undefined; schedule() }
  }
}
function onVisibilityChange() {
  if (isVisible()) { schedule(0); return }
  generation++
  controller?.abort(); controller = undefined; polling = false
  if (timer) clearTimeout(timer)
  timer = undefined
}

// One shared batch request for visible rows, independent of upstream quota queries.
export function subscribeCodexTicketStatuses(accountId: number, listener: Listener): () => void {
  let listeners = subscribers.get(accountId)
  if (!listeners) { listeners = new Set(); subscribers.set(accountId, listeners) }
  listeners.add(listener)
  if (subscribers.size === 1 && listeners.size === 1) document.addEventListener('visibilitychange', onVisibilityChange)
  if (!polling) schedule(0)
  return () => {
    const current = subscribers.get(accountId)
    current?.delete(listener)
    if (current?.size === 0) subscribers.delete(accountId)
    if (!subscribers.size) {
      generation++
      controller?.abort(); controller = undefined; polling = false
      if (timer) clearTimeout(timer)
      timer = undefined
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }
}
