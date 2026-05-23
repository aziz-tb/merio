<template>
  <div class="rounded-md border border-border overflow-hidden">
    <div class="grid grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] gap-3 px-4 py-2 text-xs font-medium text-muted-foreground bg-muted/40">
      <div>{{ t('globals.terms.tag') }}</div>
      <div>{{ t('admin.tags.type') }}</div>
      <div>{{ t('admin.detection.mode') }}</div>
      <div>{{ t('admin.tags.modified') }}</div>
      <div>{{ t('admin.tags.modifiedBy') }}</div>
    </div>

    <div v-if="grouped.length === 0" class="px-4 py-8 text-sm text-muted-foreground text-center">
      {{ t('globals.messages.noResultsFound') }}
    </div>

    <div v-for="group in grouped" :key="group.parent?.id || 'root'" class="border-t border-border first:border-t-0">
      <button
        v-if="group.parent"
        class="w-full grid grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] gap-3 px-4 py-2 text-left hover:bg-muted/40"
        @click="toggle(group.parent.id)"
      >
        <div class="flex items-center gap-2 font-medium">
          <ChevronRight class="size-4 transition-transform" :class="{ 'rotate-90': openGroups[group.parent.id] }" />
          <TagPill :tag="group.parent" />
        </div>
        <div class="text-sm text-muted-foreground">{{ group.parent.type }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.detection?.mode || 'manual_only' }}</div>
        <div class="text-sm text-muted-foreground">{{ formatDate(group.parent.updated_at) }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.updated_by_name || '-' }}</div>
      </button>

      <button
        v-if="group.parent"
        class="w-full grid grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] gap-3 px-4 py-2 text-left hover:bg-muted/40"
        @click="$emit('edit', group.parent)"
      >
        <div class="pl-6"><TagPill :tag="group.parent" /></div>
        <div class="text-sm text-muted-foreground">{{ group.parent.type }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.detection?.mode || 'manual_only' }}</div>
        <div class="text-sm text-muted-foreground">{{ formatDate(group.parent.updated_at) }}</div>
        <div class="text-sm text-muted-foreground">{{ group.parent.updated_by_name || '-' }}</div>
      </button>

      <div v-if="!group.parent || openGroups[group.parent.id]">
        <button
          v-for="tag in group.children"
          :key="tag.id"
          class="w-full grid grid-cols-[1.5fr_0.7fr_0.9fr_1fr_1fr] gap-3 px-4 py-2 text-left hover:bg-muted/40"
          @click="$emit('edit', tag)"
        >
          <div :class="group.parent ? 'pl-10' : ''">
            <TagPill :tag="tag" />
          </div>
          <div class="text-sm text-muted-foreground">{{ tag.type }}</div>
          <div class="text-sm text-muted-foreground">{{ tag.detection?.mode || 'manual_only' }}</div>
          <div class="text-sm text-muted-foreground">{{ formatDate(tag.updated_at) }}</div>
          <div class="text-sm text-muted-foreground">{{ tag.updated_by_name || '-' }}</div>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
import { format } from 'date-fns'

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
  const parentIDs = new Set(props.tags.map((tag) => idValue(tag.parent_id)).filter(Boolean))
  const root = []
  const groups = []

  for (const parentID of parentIDs) {
    const parent = byID.get(parentID)
    if (!parent) continue
    groups.push({
      parent,
      children: props.tags.filter((tag) => idValue(tag.parent_id) === parentID && tag.id !== parentID)
    })
    if (openGroups[parentID] === undefined) openGroups[parentID] = true
  }

  for (const tag of props.tags) {
    if (!idValue(tag.parent_id) && !parentIDs.has(tag.id)) {
      root.push(tag)
    }
  }
  if (root.length > 0) groups.unshift({ parent: null, children: root })
  return groups
})

const toggle = (id) => {
  openGroups[id] = !openGroups[id]
}

const formatDate = (value) => {
  if (!value) return '-'
  return format(new Date(value), 'PPp')
}

const colorClasses = {
  grey: 'bg-secondary text-secondary-foreground',
  green: 'bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-100',
  red: 'bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-100',
  blue: 'bg-blue-100 text-blue-900 dark:bg-blue-950 dark:text-blue-100',
  yellow: 'bg-yellow-100 text-yellow-900 dark:bg-yellow-950 dark:text-yellow-100',
  purple: 'bg-purple-100 text-purple-900 dark:bg-purple-950 dark:text-purple-100',
  orange: 'bg-orange-100 text-orange-900 dark:bg-orange-950 dark:text-orange-100',
  teal: 'bg-teal-100 text-teal-900 dark:bg-teal-950 dark:text-teal-100'
}

const TagPill = defineComponent({
  props: { tag: { type: Object, required: true } },
  setup (props) {
    return () => h('span', {
      class: [
        'inline-flex min-h-5 max-w-full items-center rounded-md px-2 py-0.5 text-xs font-medium',
        colorClasses[props.tag.color] || colorClasses.grey
      ]
    }, props.tag.name)
  }
})
</script>
