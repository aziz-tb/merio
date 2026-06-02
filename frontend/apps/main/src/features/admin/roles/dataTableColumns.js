import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'
import { Roles } from '@/constants/user'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-role', params: { id: row.id } })
    }),
    {
      accessorKey: 'description',
      header: () => t('globals.terms.description'),
      cell: ({ row }) => row.getValue('description')
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
      cell: ({ row }) => {
        const items = [
          {
            label: t('globals.messages.edit'),
            to: { name: 'edit-role', params: { id: row.original.id } }
          }
        ]

        if (!Roles.includes(row.original.name)) {
          items.push({
            label: t('globals.messages.delete'),
            destructive: true,
            confirm: t('role.deletionConfirmation'),
            onClick: () => onDelete?.(row.original)
          })
        }

        return h('div', { class: 'flex justify-end' },
          h(DataTableRowActions, { items })
        )
      }
    }
  ]
}
