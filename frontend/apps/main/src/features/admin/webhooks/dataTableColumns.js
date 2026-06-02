import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { Badge } from '@shared-ui/components/ui/badge'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete, onToggle, onTest } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-webhook', params: { id: row.id } })
    }),
    {
      accessorKey: 'url',
      header: () => t('globals.terms.url'),
      cell: ({ row }) => {
        const url = row.getValue('url')
        return h('div', { class: 'font-mono mt-1 max-w-sm truncate' }, url)
      }
    },
    {
      accessorKey: 'events',
      enableGlobalFilter: false,
      header: () => t('globals.terms.event', 2),
      meta: { align: 'center' },
      cell: ({ row }) => {
        const events = row.getValue('events')
        return h(
          Badge,
          { variant: 'secondary', class: 'text-xs' },
          () => `${events.length} ${t('globals.terms.event', 2)}`
        )
      },
      sortingFn: (rowA, rowB) => {
        const a = rowA.original.events?.length || 0
        const b = rowB.original.events?.length || 0
        return a - b
      }
    },
    {
      accessorKey: 'is_active',
      enableGlobalFilter: false,
      header: () => t('globals.terms.status'),
      meta: { align: 'center' },
      cell: ({ row }) => {
        const isActive = row.getValue('is_active')
        return h(
          Badge,
          {
            variant: isActive ? 'default' : 'secondary',
            class: 'text-xs'
          },
          () => isActive ? t('globals.terms.active') : t('globals.terms.inactive')
        )
      }
    },
    dateTimeCol(t, 'created_at'),
    {
      ...actionColumn,
      size: 72,
      meta: {
        ...actionColumn.meta,
        align: 'right',
        width: 72
      },
      cell: ({ row }) =>
        h('div', { class: 'flex justify-end' },
          h(DataTableRowActions, {
            items: [
              {
                label: t('globals.messages.edit'),
                to: { name: 'edit-webhook', params: { id: row.original.id } }
              },
              {
                label: row.original.is_active ? t('globals.messages.disable') : t('globals.messages.enable'),
                onClick: () => onToggle?.(row.original)
              },
              {
                label: t('webhook.sendTest'),
                onClick: () => onTest?.(row.original)
              },
              { separator: true },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('confirm.deleteWebhook'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
