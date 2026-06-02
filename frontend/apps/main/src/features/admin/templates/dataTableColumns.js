import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

const actionColumnFor = (t, onDelete) => {
  const actionColumn = actionsCol()

  return {
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
          to: { name: 'edit-template', params: { id: row.original.id } }
        }
      ]

      if (row.original.type !== 'email_notification') {
        items.push({
          label: t('globals.messages.delete'),
          destructive: true,
          confirm: t('template.deletionConfirmation'),
          onClick: () => onDelete?.(row.original)
        })
      }

      return h('div', { class: 'flex justify-end' },
        h(DataTableRowActions, { items })
      )
    }
  }
}

export const createOutgoingEmailTableColumns = (t, { onDelete } = {}) => [
  linkCol(t, 'name', {
    to: (row) => ({ name: 'edit-template', params: { id: row.id } })
  }),
  {
    accessorKey: 'is_default',
    enableGlobalFilter: false,
    header: () => t('globals.terms.default'),
    meta: { align: 'center' },
    cell: ({ row }) => {
      const isDefault = row.getValue('is_default')

      return h('input', {
        type: 'checkbox',
        checked: isDefault,
        disabled: true
      })
    }
  },
  dateTimeCol(t, 'created_at'),
  actionColumnFor(t, onDelete)
]

export const createEmailNotificationTableColumns = (t, { onDelete } = {}) => [
  linkCol(t, 'name', {
    to: (row) => ({ name: 'edit-template', params: { id: row.id } })
  }),

  dateTimeCol(t, 'created_at'),
  actionColumnFor(t, onDelete)
]
