// Knowledge articles list — column definitions for the unified DataTable.
//
// Read-B pattern: no actions column (per-row + bulk actions both live
// in DataTable's items-bar #selected-actions slot, rendered from
// KnowledgeList).
//
// Title is a single-line link to the editor — no subtitle, no
// folder-path. Folder and Language are first-class columns.

import { h } from 'vue'
import { RouterLink } from 'vue-router'
import { badgeCol, dateCol, textCol } from '@/components/datatable/helpers.js'

export const createColumns = (t, { folderById = {} } = {}) => [
  // Title — primary navigation. Single line, always shown.
  {
    accessorKey: 'title',
    enableHiding: false,
    header: () => h('div', { class: 'text-left' }, t('globals.terms.title')),
    meta: { align: 'left', label: t('globals.terms.title') },
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-left' },
        h(
          RouterLink,
          {
            to: { name: 'edit-article', params: { id: row.original.id } },
            class: 'text-primary hover:underline font-medium',
          },
          () => row.getValue('title')
        )
      ),
  },

  // Folder — just the folder name, no path. accessorFn lets sort +
  // global filter operate on the resolved name rather than the id.
  {
    id: 'folder',
    accessorFn: (row) => folderById[row.folder_id]?.name ?? '',
    header: () =>
      h('div', { class: 'text-left' }, t('admin.knowledge.folder.label')),
    meta: { align: 'left', label: t('admin.knowledge.folder.label') },
    cell: ({ row }) => {
      const name = folderById[row.original.folder_id]?.name
      return h(
        'div',
        { class: 'text-left text-sm text-muted-foreground' },
        name || '—'
      )
    },
  },

  // Language — uppercase locale code (EN / DE / FR / …).
  textCol(t, 'locale', {
    headerKey: 'admin.knowledge.locale.label',
    formatter: (v) => v?.toUpperCase() ?? '—',
  }),

  // Status — badge.
  badgeCol(t, 'status', {
    variant: (v) => (v === 'published' ? 'default' : 'secondary'),
    label: (v, t) => t(`admin.knowledge.status.${v}`),
  }),

  // Classification — localized enum label.
  textCol(t, 'classification', {
    headerKey: 'admin.knowledge.classification.label',
    formatter: (v, t) => t(`admin.knowledge.classification.${v}`),
  }),

  // Updated — smart date.
  dateCol(t, 'updated_at'),
]