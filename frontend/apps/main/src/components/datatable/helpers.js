// Column factory helpers for the unified DataTable.
//
// Reduces the boilerplate in per-feature column files from ~80 lines
// of manual h() calls to ~25 lines of declarative helper invocations.
// All helpers set meta.label so DataTable's column-visibility menu
// shows a human-readable label rather than the accessor key.

import { h } from 'vue'
import { RouterLink } from 'vue-router'
import { format, formatDistanceToNow, differenceInCalendarDays } from 'date-fns'
import { Badge } from '@shared-ui/components/ui/badge'

/**
 * linkCol — first-column router link.
 *
 *   linkCol(t, 'name', { to: (row) => ({ name: 'edit-webhook', params: { id: row.id } }) })
 */
export function linkCol(t, key, { to, headerKey, ...rest } = {}) {
  const label = t(headerKey || `globals.terms.${key}`)
  return {
    accessorKey: key,
    header: () => h('div', { class: 'text-center' }, label),
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-center' },
        h(
          RouterLink,
          {
            to: to(row.original),
            class: 'text-primary hover:underline',
          },
          () => row.getValue(key)
        )
      ),
    meta: { label },
    ...rest,
  }
}

/**
 * textCol — generic text column. Optional formatter for value
 * transformation (uppercase locale codes, i18n-keyed enums, etc.).
 *
 *   textCol(t, 'locale', { headerKey: 'admin.knowledge.locale.label',
 *     formatter: (v) => v?.toUpperCase() ?? '—' })
 */
export function textCol(t, key, { headerKey, formatter, ...rest } = {}) {
  const label = t(headerKey || `globals.terms.${snakeToCamel(key)}`)
  return {
    accessorKey: key,
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, label),
    cell: ({ row }) => {
      const v = row.getValue(key)
      const display = formatter ? formatter(v, t) : v ?? '—'
      return h(
        'div',
        { class: 'text-center text-sm text-muted-foreground' },
        display
      )
    },
    meta: { label },
    ...rest,
  }
}

/**
 * dateCol — smart date column. Default mode 'smart' renders relative
 * for <7 days, absolute beyond, full timestamp on hover tooltip.
 * Date columns are excluded from global search by default.
 */
export function dateCol(t, key, { mode = 'smart', headerKey, ...rest } = {}) {
  const label = t(headerKey || `globals.terms.${snakeToCamel(key)}`)
  return {
    accessorKey: key,
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, label),
    cell: ({ row }) => {
      const v = row.getValue(key)
      if (!v) {
        return h('div', { class: 'text-center text-muted-foreground' }, '—')
      }
      const d = new Date(v)
      const display =
        mode === 'absolute'
          ? format(d, 'PP')
          : mode === 'relative'
            ? formatDistanceToNow(d, { addSuffix: true })
            : smartFormat(d)
      const tooltip = format(d, 'PPpp')
      return h(
        'div',
        { class: 'text-center text-muted-foreground', title: tooltip },
        display
      )
    },
    meta: { label },
    ...rest,
  }
}

function smartFormat(d) {
  const days = Math.abs(differenceInCalendarDays(new Date(), d))
  return days < 7
    ? formatDistanceToNow(d, { addSuffix: true })
    : format(d, 'PP')
}

/**
 * badgeCol — status / boolean column rendered as a Badge. Excluded
 * from global search.
 */
export function badgeCol(t, key, { variant, label: labelFn, headerKey, ...rest } = {}) {
  const headerLabel = t(headerKey || `globals.terms.${key}`)
  return {
    accessorKey: key,
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, headerLabel),
    cell: ({ row }) => {
      const v = row.getValue(key)
      const resolvedVariant =
        typeof variant === 'function' ? variant(v) : variant || 'secondary'
      const resolvedLabel =
        typeof labelFn === 'function' ? labelFn(v, t) : String(v ?? '')
      return h(
        'div',
        { class: 'text-center' },
        h(
          Badge,
          { variant: resolvedVariant, class: 'text-xs' },
          () => resolvedLabel
        )
      )
    },
    meta: { label: headerLabel },
    ...rest,
  }
}

/**
 * actionsCol — declares a per-row actions column for tables that
 * still use the per-row dropdown pattern. Read-B-style pages
 * (actions in the items bar) don't need this — they omit the
 * actions column entirely.
 */
export function actionsCol() {
  return {
    id: 'actions',
    enableHiding: false,
    enableSorting: false,
    enableGlobalFilter: false,
    header: () => null,
    cell: () => null,
    meta: { isActions: true, align: 'center' },
  }
}

function snakeToCamel(s) {
  return String(s).replace(/_([a-z])/g, (_, c) => c.toUpperCase())
}