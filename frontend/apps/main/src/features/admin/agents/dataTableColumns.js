import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'first_name', {
      headerKey: 'globals.terms.firstName',
      to: (row) => ({ name: 'edit-agent', params: { id: row.id } })
    }),
    {
      accessorKey: 'last_name',
      header: () => t('globals.terms.lastName'),
      cell: ({ row }) => row.getValue('last_name')
    },
    {
      accessorKey: 'enabled',
      enableGlobalFilter: false,
      header: () => t('globals.terms.enabled'),
      cell: ({ row }) => row.getValue('enabled') ? t('globals.messages.yes') : t('globals.messages.no')
    },
    {
      accessorKey: 'email',
      header: () => t('globals.terms.email'),
      cell: ({ row }) => row.getValue('email')
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
                to: { name: 'edit-agent', params: { id: row.original.id } }
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('admin.agent.deleteConfirmation'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
