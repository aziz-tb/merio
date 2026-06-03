import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import StatusPill from '@main/components/status/StatusPill.vue'
import { actionsCol, dateTimeCol } from '@/components/datatable/helpers.js'
import { CONVERSATION_DEFAULT_STATUSES_LIST } from '@/constants/conversation.js'
import { getStatusDisplayLabel } from '@main/utils/statusVisuals'

export const createColumns = (t, { onEdit, onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    {
      accessorKey: 'name',
      header: () => t('globals.terms.status'),
      cell: ({ row }) => {
        const name = row.getValue('name')
        const isDefault = CONVERSATION_DEFAULT_STATUSES_LIST.includes(name)
        const label = isDefault ? getStatusDisplayLabel(row.original, t) : name
        const isClickable = onEdit && !isDefault

        return h(StatusPill, {
          as: isClickable ? 'button' : 'span',
          ...(isClickable ? { type: 'button' } : {}),
          status: row.original,
          label,
          size: 'md',
          class: [
            'max-w-full',
            isClickable
              ? 'cursor-pointer transition-opacity hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-1'
              : ''
          ],
          onClick: isClickable ? () => onEdit(row.original) : undefined
        })
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
