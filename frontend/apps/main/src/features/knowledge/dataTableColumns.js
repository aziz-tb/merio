import { h } from 'vue'
import { formatDistanceToNow } from 'date-fns'
import { Badge } from '@shared-ui/components/ui/badge'
import dropdown from './dataTableDropdown.vue'

// Build the chain "Parent / Child / Grandchild" for a folder id from the
// flat folders array (cheap; tree depth in practice is shallow).
const buildFolderPath = (folderId, byId) => {
  const parts = []
  let cur = byId[folderId]
  while (cur) {
    parts.unshift(cur.name)
    cur = cur.parent_id != null ? byId[cur.parent_id] : null
  }
  return parts.join(' / ')
}

export const createColumns = (t, { onEdit, folderById = {} } = {}) => [
  {
    accessorKey: 'title',
    header: () => h('div', { class: 'text-left' }, t('globals.terms.title')),
    cell: ({ row }) => {
      const article = row.original
      const folderPath = buildFolderPath(article.folder_id, folderById)
      // Secondary line: folder path · locale. AI-grounded flag is shown in
      // the article form itself (not in the row, to avoid re-using/inventing
      // an i18n key for a row-level chip).
      const segments = [folderPath, article.locale?.toUpperCase()]
      const secondary = segments.filter(Boolean).join(' · ')

      return h('div', { class: 'text-left' }, [
        onEdit
          ? h(
              'button',
              {
                type: 'button',
                class:
                  'block text-foreground hover:text-primary hover:underline cursor-pointer font-medium text-left',
                onClick: () => onEdit(article)
              },
              row.getValue('title')
            )
          : h('span', { class: 'block font-medium' }, row.getValue('title')),
        secondary
          ? h(
              'span',
              { class: 'block text-xs text-muted-foreground mt-0.5 truncate' },
              secondary
            )
          : null
      ])
    }
  },
  {
    accessorKey: 'status',
    header: () => h('div', { class: 'text-center' }, t('globals.terms.status')),
    cell: ({ row }) => {
      const v = row.getValue('status')
      return h(
        'div',
        { class: 'text-center' },
        h(
          Badge,
          { variant: v === 'published' ? 'default' : 'secondary' },
          { default: () => t(`admin.knowledge.status.${v}`) }
        )
      )
    }
  },
  {
    accessorKey: 'classification',
    header: () => h('div', { class: 'text-center' }, t('admin.knowledge.classification.label')),
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-center' },
        t(`admin.knowledge.classification.${row.getValue('classification')}`)
      )
  },
  {
    accessorKey: 'updated_at',
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, t('globals.terms.updatedAt')),
    cell: ({ row }) => {
      const d = new Date(row.getValue('updated_at'))
      return h(
        'div',
        { class: 'text-center text-muted-foreground' },
        formatDistanceToNow(d, { addSuffix: true })
      )
    }
  },
  {
    id: 'actions',
    enableHiding: false,
    enableSorting: false,
    cell: ({ row }) => {
      const article = row.original
      return h('div', { class: 'relative' }, h(dropdown, { article }))
    }
  }
]
