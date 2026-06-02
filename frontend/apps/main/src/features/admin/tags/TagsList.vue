<template>
  <DataTable
    :columns="columns"
    :data="rows"
    :loading="loading"
    :searchable="true"
    :column-visibility="true"
    :column-draggable="true"
    :column-resizable="true"
    header-menu
    storage-key="admin-tags"
    @row-click="(tag) => emit('edit', tag)"
  />
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from './dataTableColumns.js'

const props = defineProps({
  tags: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['edit', 'delete'])
const { t } = useI18n()

const columns = computed(() =>
  createColumns(t, {
    onEdit: (tag) => emit('edit', tag),
    onDelete: (tag) => emit('delete', tag)
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

  const parentTags = rootTags.filter((tag) => childrenByParent.has(tag.id))
  const leafRootTags = rootTags.filter((tag) => !childrenByParent.has(tag.id))
  const flattened = []

  for (const parent of parentTags) {
    const children = childrenByParent.get(parent.id) || []
    flattened.push(decorateTag(parent, { childCount: children.length }))
    for (const child of children) {
      flattened.push(decorateTag(child, {
        depth: 1,
        parentName: parent.name
      }))
    }
  }

  for (const tag of leafRootTags) {
    flattened.push(decorateTag(tag))
  }

  return flattened
})

function decorateTag(tag, options = {}) {
  return {
    ...tag,
    _depth: options.depth || 0,
    _parentName: options.parentName || '',
    _childCount: options.childCount || 0,
    _detectionMode: tag.detection?.mode || 'manual_only'
  }
}

function idValue(value) {
  if (value && typeof value === 'object') {
    return value.Valid ? value.Int : null
  }
  return value || null
}
</script>
