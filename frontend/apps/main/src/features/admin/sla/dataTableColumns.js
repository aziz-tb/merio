import { h } from 'vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { onDelete } = {}) => {
    const actionColumn = actionsCol()

    return [
        linkCol(t, 'name', {
            to: (row) => ({ name: 'edit-sla', params: { id: row.id } })
        }),
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
                                to: { name: 'edit-sla', params: { id: row.original.id } }
                            },
                            {
                                label: t('globals.messages.delete'),
                                destructive: true,
                                confirm: t('sla.deletionConfirmation'),
                                onClick: () => onDelete?.(row.original)
                            }
                        ]
                    })
                )
        }
    ]
}
