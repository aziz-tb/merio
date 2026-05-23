import { h } from 'vue'
import { RouterLink } from 'vue-router'
import dropdown from './dataTableDropdown.vue'
import { format } from 'date-fns'
import { Badge } from '@shared-ui/components/ui/badge'

export const createColumns = (t) => [
  {
    accessorKey: 'name',
    header: () => h('div', { class: 'text-center' }, t('globals.terms.name')),
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-center' },
        h(
          RouterLink,
          {
            to: { name: 'edit-ai-agent', params: { id: row.original.id } },
            class: 'text-primary hover:underline'
          },
          () => row.getValue('name')
        )
      )
  },
  {
    accessorKey: 'slug',
    header: () => h('div', { class: 'text-center' }, t('globals.terms.slug')),
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-center font-mono text-sm text-muted-foreground' },
        row.getValue('slug')
      )
  },
  {
    accessorKey: 'enabled',
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, t('globals.terms.status')),
    cell: ({ row }) => {
      const enabled = row.getValue('enabled')
      return h('div', { class: 'text-center' }, [
        h(
          Badge,
          { variant: enabled ? 'default' : 'secondary', class: 'text-xs' },
          () => (enabled ? t('globals.terms.enabled') : t('globals.terms.disabled'))
        )
      ])
    }
  },
  {
    accessorKey: 'updated_at',
    enableGlobalFilter: false,
    header: () => h('div', { class: 'text-center' }, t('globals.terms.updatedAt') || 'Updated'),
    cell: ({ row }) =>
      h(
        'div',
        { class: 'text-center text-sm' },
        format(new Date(row.getValue('updated_at')), 'PPpp')
      )
  },
  {
    id: 'actions',
    enableHiding: false,
    enableSorting: false,
    cell: ({ row }) => h('div', { class: 'relative' }, h(dropdown, { agent: row.original }))
  }
]
