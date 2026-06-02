import { h } from 'vue'
import TagPill from '@shared-ui/components/TagPill.vue'
import DataTableRowActions from '@/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol } from '@/components/datatable/helpers.js'

const TYPE_KEY = {
  skill: 'admin.tags.typeSkill',
  topic: 'admin.tags.typeTopic',
  contact: 'admin.tags.typeContact',
  other: 'admin.tags.typeOther'
}

const VISIBILITY_KEY = {
  public: 'admin.tags.visibilityPublic',
  private: 'admin.tags.visibilityPrivate',
  disabled: 'admin.tags.visibilityDisabled'
}

export const createColumns = (t, { onEdit, onDelete } = {}) => {
  const actionColumn = actionsCol()

  return [
    {
      accessorKey: 'name',
      header: () => t('admin.tags.tag'),
      size: 260,
      meta: { label: t('admin.tags.tag'), width: 260 },
      cell: ({ row }) => {
        const tag = row.original
        return h('button', {
          type: 'button',
          class: [
            'flex max-w-full items-center gap-2 text-left',
            tag._depth ? 'pl-7' : ''
          ],
          onClick: (event) => {
            event.stopPropagation()
            onEdit?.(tag)
          }
        }, [
          h(TagPill, { tag, class: 'max-w-full' }),
          tag._childCount
            ? h('span', { class: 'shrink-0 text-xs text-muted-foreground' }, `(${tag._childCount})`)
            : null
        ])
      }
    },
    {
      accessorKey: '_parentName',
      header: () => t('admin.tags.parent'),
      size: 180,
      meta: { label: t('admin.tags.parent'), width: 180 },
      cell: ({ row }) =>
        h('div', { class: 'text-sm text-muted-foreground' }, row.getValue('_parentName') || '-')
    },
    {
      accessorKey: 'type',
      header: () => t('admin.tags.type'),
      size: 120,
      meta: { label: t('admin.tags.type'), width: 120 },
      cell: ({ row }) =>
        h('div', { class: 'text-sm text-muted-foreground' },
          t(TYPE_KEY[row.getValue('type')] || TYPE_KEY.skill)
        )
    },
    {
      accessorKey: 'visibility',
      header: () => t('admin.tags.visibility'),
      size: 130,
      meta: { label: t('admin.tags.visibility'), width: 130 },
      cell: ({ row }) =>
        h('div', { class: 'text-sm text-muted-foreground' },
          t(VISIBILITY_KEY[row.getValue('visibility')] || VISIBILITY_KEY.public)
        )
    },
    {
      accessorKey: '_detectionMode',
      header: () => t('admin.tags.detectionMode'),
      size: 150,
      meta: { label: t('admin.tags.detectionMode'), width: 150 },
      cell: ({ row }) =>
        h('div', { class: 'text-sm text-muted-foreground' }, formatMode(row.getValue('_detectionMode')))
    },
    dateTimeCol(t, 'updated_at', {
      headerKey: 'admin.tags.lastModified',
      meta: { width: 180 }
    }),
    {
      accessorKey: 'updated_by_name',
      header: () => t('admin.tags.modifiedBy'),
      size: 160,
      meta: { label: t('admin.tags.modifiedBy'), width: 160 },
      cell: ({ row }) =>
        h('div', { class: 'text-sm text-muted-foreground' }, row.getValue('updated_by_name') || '-')
    },
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
                confirm: t('admin.tags.deleteConfirmation'),
                onClick: () => onDelete?.(row.original)
              }
            ]
          })
        )
    }
  ]
}

function formatMode(value) {
  return String(value || 'manual_only').replaceAll('_', ' ')
}
