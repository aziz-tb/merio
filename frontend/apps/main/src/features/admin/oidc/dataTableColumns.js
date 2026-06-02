import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    linkCol(t, 'name', {
      to: (row) => ({ name: 'edit-sso', params: { id: row.id } })
    }),
    {
      accessorKey: 'provider',
      header: () => t('globals.terms.provider'),
      cell: ({ row }) => row.getValue('provider')
    },
    {
      accessorKey: 'enabled',
      enableGlobalFilter: false,
      header: () => t('globals.terms.enabled'),
      cell: ({ row }) => {
        const enabled = row.getValue('enabled')
        return enabled ? t('globals.messages.yes') : t('globals.messages.no')
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
                to: { name: 'edit-sso', params: { id: row.original.id } }
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('confirm.deleteSso'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
