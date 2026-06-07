import { format } from 'date-fns'

/**
 * Default date+time format used across the platform.
 * Example: "May 9, 2026, 14:45"
 */
export function formatDateTime(value) {
  const d = toDate(value)
  if (!d) return ''
  return format(d, 'd MMM, HH:mm')
}

/**
 * Date+time with seconds. Used for audit/security pages where
 * exact timestamps matter.
 * Example: "May 9, 2026, 14:45:35"
 */
export function formatDateTimeWithSeconds(value) {
  const d = toDate(value)
  if (!d) return ''
  return format(d, 'PP, HH:mm:ss')
}

function toDate(value) {
  if (!value) return null
  const d = value instanceof Date ? value : new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}
