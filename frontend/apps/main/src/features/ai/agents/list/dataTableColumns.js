import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { Badge } from '@shared-ui/components/ui/badge'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-ai-agent', params: { id: row.id } })
    }),
    {
      accessorKey: 'slug',
      header: () => t('globals.terms.slug'),
      cell: ({ row }) =>
        h(
          'div',
          { class: 'font-mono text-sm text-muted-foreground' },
          row.getValue('slug')
        )
    },
    {
      accessorKey: 'enabled',
      enableGlobalFilter: false,
      header: () => t('globals.terms.status'),
      meta: { align: 'center' },
      cell: ({ row }) => {
        const enabled = row.getValue('enabled')
        return h(
          Badge,
          { variant: enabled ? 'default' : 'secondary', class: 'text-xs' },
          () => (enabled ? t('globals.terms.enabled') : t('globals.terms.disabled'))
        )
      }
    },
    dateTimeCol(t, 'updated_at'),
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
                to: { name: 'edit-ai-agent', params: { id: row.original.id } }
              },
              { separator: true },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('aiAgent.deletionConfirmation'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
