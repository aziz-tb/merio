import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-macro', params: { id: row.id } })
    }),
    {
      accessorKey: 'visibility',
      header: () => t('globals.terms.visibility'),
      cell: ({ row }) => row.getValue('visibility')
    },
    {
      accessorKey: 'usage_count',
      enableGlobalFilter: false,
      header: () => t('globals.terms.usage'),
      meta: { align: 'center' },
      cell: ({ row }) => row.getValue('usage_count')
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
                to: { name: 'edit-macro', params: { id: row.original.id } }
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('confirm.deleteMacro'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
