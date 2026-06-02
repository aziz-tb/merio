import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onEdit, onDelete } = {}) => {
    const actionColumn = actionsCol()

    return [
        {
            accessorKey: 'name',
            header: () => t('globals.terms.name'),
            cell: ({ row }) =>
                onEdit
                  ? h('span', {
                      class: 'text-primary hover:underline cursor-pointer',
                      onClick: () => onEdit(row.original)
                    }, row.getValue('name'))
                  : row.getValue('name')
        },
        {
            accessorKey: 'key',
            header: () => t('globals.terms.key'),
            cell: ({ row }) => row.getValue('key')
        },
        {
            accessorKey: 'data_type',
            header: () => t('globals.terms.type'),
            cell: ({ row }) => row.getValue('data_type')
        },
        {
            accessorKey: 'applies_to',
            header: () => t('globals.terms.appliesTo'),
            cell: ({ row }) => row.getValue('applies_to')
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
                                onClick: () => onEdit?.(row.original)
                            },
                            {
                                label: t('globals.messages.delete'),
                                destructive: true,
                                confirm: t('customAttribute.deletionConfirmation'),
                                onClick: () => onDelete?.(row.original)
                            }
                        ]
                    })
                )
        }
    ]
}
