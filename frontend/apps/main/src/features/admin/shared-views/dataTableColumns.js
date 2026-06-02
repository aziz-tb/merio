import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-shared-view', params: { id: row.id } })
    }),
    {
      accessorKey: 'visibility',
      header: () => t('globals.terms.visibility'),
      cell: ({ row }) => {
        const visibility = row.getValue('visibility')
        return visibility === 'all' ? t('sharedView.allAgents') : t('globals.terms.team')
      }
    },
    dateTimeCol(t, 'created_at'),
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
                to: { name: 'edit-shared-view', params: { id: row.original.id } }
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('confirm.deleteSharedView'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
