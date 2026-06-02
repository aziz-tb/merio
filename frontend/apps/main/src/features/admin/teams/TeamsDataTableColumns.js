import { h } from 'vue'
import { RouterLink } from 'vue-router'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    {
      accessorKey: 'name',
      header: () => t('globals.terms.name', 1),
      cell: ({ row }) => {
        const emoji = row.original.emoji
        return h(RouterLink,
          {
            to: { name: 'edit-team', params: { id: row.original.id } },
            class: 'text-primary hover:underline'
          },
          () => [emoji ? `${emoji} ` : '', row.getValue('name')]
        )
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
                to: { name: 'edit-team', params: { id: row.original.id } }
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                confirm: t('confirm.deleteTeam'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}
