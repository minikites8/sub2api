export interface UnavailableTimeFields {
  unavailable_time_enabled?: boolean
  unavailable_start?: string
  unavailable_end?: string
}

export function formatUnavailableTimeWindow(
  fields?: UnavailableTimeFields | null,
  timezoneLabel = '',
  nextDayLabel = 'next day'
): string {
  if (!fields?.unavailable_time_enabled || !fields.unavailable_start || !fields.unavailable_end) return ''
  const { unavailable_start: start, unavailable_end: end } = fields
  const endLabel = start > end ? `${nextDayLabel} ${end}` : end
  const window = `${start}–${endLabel}`
  return timezoneLabel ? `${window} (${timezoneLabel})` : window
}
