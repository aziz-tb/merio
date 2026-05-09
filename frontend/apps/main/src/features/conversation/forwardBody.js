// HTML builder for the forwarded-source blocks in Forward mode. Used both by the
// composer's non-editable preview and by processForward at send time. Keeping this
// pure (no Vue dependency) so it can be called from anywhere without round-tripping
// through Tiptap — that round-trip is exactly what stripped the email HTML before.
import { formatFullTimestamp } from '@shared-ui/utils/datetime.js'

const escape = (s) =>
  String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

const joinList = (xs) => (Array.isArray(xs) ? xs.filter(Boolean).join(', ') : '')

export const buildForwardSourceHTML = (source) => {
  const meta = source.meta || {}
  const from = joinList(meta.from)
  const to = joinList(meta.to)
  const subject = meta.subject || ''
  const date = source.created_at ? formatFullTimestamp(source.created_at) : ''
  return (
    `<div data-source-uuid="${escape(source.uuid)}" class="forwarded-block">` +
    `<p>---------- Forwarded message ----------</p>` +
    `<p><strong>From:</strong> ${escape(from)}<br>` +
    `<strong>Date:</strong> ${escape(date)}<br>` +
    `<strong>Subject:</strong> ${escape(subject)}<br>` +
    `<strong>To:</strong> ${escape(to)}</p>` +
    `<blockquote>${source.content || ''}</blockquote>` +
    `</div>`
  )
}
