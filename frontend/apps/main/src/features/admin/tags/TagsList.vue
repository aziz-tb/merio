<template>
  <DataTable
    :columns="columns"
    :data="rows"
    :loading="loading"
    :searchable="false"
    :column-visibility="true"
    :column-draggable="true"
    :column-resizable="true"
    header-menu
    storage-key="admin-tags"
    @row-click="(tag) => emit('edit', tag)"
  >
    <template #toolbar>
      <div class="relative w-72 max-w-full">
        <MagnifyingGlass class="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          v-model="searchQuery"
          :placeholder="t('globals.terms.search')"
          class="h-8 border-border/60 bg-transparent pl-9 text-xs shadow-none focus-visible:bg-background"
        />
      </div>
    </template>
  </DataTable>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MagnifyingGlass } from '@qall/icons'
import DataTable from '@main/components/datatable/DataTable.vue'
import { Input } from '@shared-ui/components/ui/input'
import { createColumns } from './dataTableColumns.js'

const props = defineProps({
  tags: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['edit', 'delete'])
const { t } = useI18n()
const searchQuery = ref('')
const expandedParentIds = ref(new Set())
const normalizedSearchQuery = computed(() => searchQuery.value.trim().toLowerCase())

const columns = computed(() =>
  createColumns(t, {
    onEdit: (tag) => emit('edit', tag),
    onDelete: (tag) => emit('delete', tag),
    onToggleParent: toggleParent,
    isExpanded: (tag) => expandedParentIds.value.has(tag.id)
  })
)

const rows = computed(() => {
  const byID = new Map(props.tags.map((tag) => [tag.id, tag]))
  const childrenByParent = new Map()
  const rootTags = []

  for (const tag of props.tags) {
    const parentID = idValue(tag.parent_id)
    if (parentID && byID.has(parentID)) {
      if (!childrenByParent.has(parentID)) childrenByParent.set(parentID, [])
      childrenByParent.get(parentID).push(tag)
    } else {
      rootTags.push(tag)
    }
  }

  const query = normalizedSearchQuery.value
  const parentTags = rootTags.filter((tag) => childrenByParent.has(tag.id))
  const leafRootTags = rootTags.filter((tag) => !childrenByParent.has(tag.id))
  const flattened = []

  for (const parent of parentTags) {
    const children = childrenByParent.get(parent.id) || []
    const parentMatches = tagMatches(parent, '', query)
    const visibleChildren = query
      ? children.filter((child) => parentMatches || tagMatches(child, parent.name, query))
      : children

    if (query && !parentMatches && visibleChildren.length === 0) {
      continue
    }

    const isExpanded = query ? true : expandedParentIds.value.has(parent.id)
    flattened.push(decorateTag(parent, {
      childCount: children.length,
      expanded: isExpanded
    }))

    if (!isExpanded) {
      continue
    }

    for (const child of visibleChildren) {
      flattened.push(decorateTag(child, {
        depth: 1,
        parentName: parent.name
      }))
    }
  }

  for (const tag of leafRootTags) {
    if (query && !tagMatches(tag, '', query)) {
      continue
    }
    flattened.push(decorateTag(tag))
  }

  return flattened
})

function toggleParent(tag) {
  if (!tag?._childCount) return
  const next = new Set(expandedParentIds.value)
  if (next.has(tag.id)) {
    next.delete(tag.id)
  } else {
    next.add(tag.id)
  }
  expandedParentIds.value = next
}

function decorateTag(tag, options = {}) {
  return {
    ...tag,
    _depth: options.depth || 0,
    _parentName: options.parentName || '',
    _childCount: options.childCount || 0,
    _isExpanded: !!options.expanded,
    _detectionMode: tag.detection?.mode || 'manual_only'
  }
}

function tagMatches(tag, parentName, query) {
  if (!query) return true
  return [
    tag.name,
    tag.description,
    tag.type,
    tag.visibility,
    tag.detection?.mode,
    tag.updated_by_name,
    parentName
  ].some((value) => String(value || '').toLowerCase().includes(query))
}

function idValue(value) {
  if (value && typeof value === 'object') {
    return value.Valid ? value.Int : null
  }
  return value || null
}
</script>
