<template>
  <div class="overflow-hidden rounded-md border border-border bg-background">
    <div class="sticky top-0 z-20 grid h-9 grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] items-center gap-3 border-b border-border bg-background px-3 text-xs font-medium text-muted-foreground/80">
      <div>{{ t('admin.tags.tag') }}</div>
      <div>{{ t('admin.tags.type') }}</div>
      <div>{{ t('admin.tags.detectionMode') }}</div>
      <div>{{ t('admin.tags.lastModified') }}</div>
      <div>{{ t('admin.tags.modifiedBy') }}</div>
    </div>

    <div v-if="grouped.length === 0" class="flex h-64 items-center justify-center px-4 text-center text-sm text-muted-foreground">
      {{ t('globals.messages.noResultsFound') }}
    </div>

    <div v-for="group in grouped" :key="group.parent?.id || 'root'" class="border-b border-border/40 last:border-b-0">
      <div
        v-if="group.parent"
        class="grid min-h-14 cursor-pointer grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] items-center gap-3 px-3 py-2 transition-colors hover:bg-muted/40"
        @click="$emit('edit', group.parent)"
      >
        <div class="flex min-w-0 items-center gap-2 font-medium">
          <button
            v-if="group.children.length > 0"
            type="button"
            class="shrink-0 rounded p-0.5 hover:bg-muted"
            @click.stop="toggle(group.parent.id)"
          >
            <ChevronRight class="size-4 transition-transform" :class="{ 'rotate-90': openGroups[group.parent.id] }" />
          </button>
          <span v-else class="size-5 shrink-0" />
          <TagPill :tag="group.parent" />
        </div>
        <div class="text-sm text-muted-foreground">{{ group.parent.type }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.detection?.mode || 'manual_only' }}</div>
        <div class="text-sm text-muted-foreground">{{ formatDate(group.parent.updated_at) }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.updated_by_name || '-' }}</div>
      </div>

      <div
        v-for="child in (group.parent && openGroups[group.parent.id] ? group.children : [])"
        :key="child.id"
        class="grid min-h-14 cursor-pointer grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] items-center gap-3 border-t border-border/40 px-3 py-2 transition-colors hover:bg-muted/40"
        @click="$emit('edit', child)"
      >
        <div class="pl-7">
          <TagPill :tag="child" />
        </div>
        <div class="text-sm text-muted-foreground">{{ child.type }}</div>
        <div class="text-sm text-muted-foreground">{{ child.detection?.mode || 'manual_only' }}</div>
        <div class="text-sm text-muted-foreground">{{ formatDate(child.updated_at) }}</div>
        <div class="text-sm text-muted-foreground">{{ child.updated_by_name || '-' }}</div>
      </div>

      <template v-if="!group.parent">
        <div
          v-for="tag in group.children"
          :key="tag.id"
          class="grid min-h-14 cursor-pointer grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] items-center gap-3 border-t border-border/40 px-3 py-2 transition-colors first:border-t-0 hover:bg-muted/40"
          @click="$emit('edit', tag)"
        >
          <div class="flex min-w-0 items-center gap-2">
            <span class="size-5 shrink-0" />
            <TagPill :tag="tag" />
          </div>
          <div class="text-sm text-muted-foreground">{{ tag.type }}</div>
          <div class="text-sm text-muted-foreground">{{ tag.detection?.mode || 'manual_only' }}</div>
          <div class="text-sm text-muted-foreground">{{ formatDate(tag.updated_at) }}</div>
          <div class="text-sm text-muted-foreground">{{ tag.updated_by_name || '-' }}</div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
import TagPill from '@shared-ui/components/TagPill.vue'
import { formatDateTime } from '@main/utils/date'

const props = defineProps({
  tags: { type: Array, default: () => [] }
})
defineEmits(['edit'])

const { t } = useI18n()
const openGroups = reactive({})

const idValue = (value) => {
  if (value && typeof value === 'object') {
    return value.Valid ? value.Int : null
  }
  return value || null
}

const grouped = computed(() => {
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
  const groups = []

  for (const parent of parentTags) {
    groups.push({
      parent,
      children: childrenByParent.get(parent.id) || []
    })
  }

  if (leafRootTags.length > 0) groups.push({ parent: null, children: leafRootTags })
  return groups
})

watch(grouped, (groups) => {
  for (const group of groups) {
    if (group.parent && openGroups[group.parent.id] === undefined) {
      openGroups[group.parent.id] = true
    }
  }
}, { immediate: true })

const toggle = (id) => {
  openGroups[id] = !openGroups[id]
}

const formatDate = (value) => {
  if (!value) return '-'
  return formatDateTime(value) || '-'
}
</script>
