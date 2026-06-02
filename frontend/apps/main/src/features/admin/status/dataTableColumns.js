import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol } from '@/components/datatable/helpers.js'
import { CONVERSATION_DEFAULT_STATUSES_LIST } from '@/constants/conversation.js'

const DEFAULT_STATUS_KEY = {
  Open: 'globals.terms.open',
  Snoozed: 'globals.terms.snoozed',
  Resolved: 'globals.terms.resolved',
  Closed: 'globals.terms.closed'
}

export const createColumns = (t, { onEdit, onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    {
      accessorKey: 'name',
      header: () => t('globals.terms.name'),
      cell: ({ row }) => {
        const name = row.getValue('name')
        const isDefault = CONVERSATION_DEFAULT_STATUSES_LIST.includes(name)
        const label = isDefault ? t(DEFAULT_STATUS_KEY[name]) : name

        return onEdit && !isDefault
          ? h('span', {
              class: 'text-primary hover:underline cursor-pointer',
              onClick: () => onEdit(row.original)
            }, label)
          : label
      }
    },
    {
      accessorKey: 'category',
      header: () => t('globals.terms.category'),
      cell: ({ row }) => t(`globals.terms.${row.getValue('category')}`)
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
        const isDefault = CONVERSATION_DEFAULT_STATUSES_LIST.includes(row.original.name)

        return h('div', { class: 'flex justify-end' },
          h(DataTableRowActions, {
            items: [
              {
                label: t('globals.messages.edit'),
                disabled: isDefault,
                onClick: () => onEdit?.(row.original)
              },
              {
                label: t('globals.messages.delete'),
                destructive: true,
                disabled: isDefault,
                confirm: t('status.deletionConfirmation'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
      }
    }
  ]
}
