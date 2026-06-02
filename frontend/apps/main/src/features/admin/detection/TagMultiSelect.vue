<template>
  <div class="rounded-md border border-input bg-background">
    <div class="border-b border-border p-2">
      <Input v-model="query" :placeholder="t('globals.messages.search')" />
    </div>
    <div class="max-h-56 overflow-auto p-2">
      <div v-if="filteredGroups.length === 0" class="px-2 py-4 text-sm text-muted-foreground">
        {{ $t('globals.messages.noResultsFound') }}
      </div>
      <div v-for="group in filteredGroups" :key="group.id" class="space-y-1">
        <div v-if="group.label" class="px-2 pt-2 text-xs font-medium text-muted-foreground">
          {{ group.label }}
        </div>
        <label
          v-for="tag in group.tags"
          :key="tag.name"
          class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted"
        >
          <Checkbox :checked="selected.includes(tag.name)" @update:checked="toggle(tag.name)" />
          <TagPill :tag="tag" />
        </label>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Input } from '@shared-ui/components/ui/input'
import TagPill from '@shared-ui/components/TagPill.vue'
import { useTagStore } from '@/stores/tag'

const selected = defineModel({ required: false, default: () => [] })
const { t } = useI18n()
const tagStore = useTagStore()
const query = ref('')

onMounted(() => {
  tagStore.fetchTags()
})

const tags = computed(() => tagStore.tags || [])

const groupedTags = computed(() => {
  const hasParents = tags.value.some((tag) => tag.parent_id)
  if (!hasParents) {
    return [{ id: 'flat', label: '', tags: tags.value }]
  }

  const parents = tags.value.filter((tag) => !tag.parent_id)
  const parentGroups = parents.map((parent) => ({
    id: parent.id,
    label: parent.name,
    tags: tags.value.filter((tag) => tag.parent_id === parent.id)
  }))
  const orphanTags = tags.value.filter((tag) => tag.parent_id && !parents.some((p) => p.id === tag.parent_id))
  return [{ id: 'top', label: '', tags: parents.concat(orphanTags) }, ...parentGroups]
    .filter((group) => group.tags.length > 0)
})

const filteredGroups = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return groupedTags.value
  return groupedTags.value
    .map((group) => ({
      ...group,
      tags: group.tags.filter((tag) => tag.name.toLowerCase().includes(q))
    }))
    .filter((group) => group.tags.length > 0)
})

const toggle = (name) => {
  if (selected.value.includes(name)) {
    selected.value = selected.value.filter((tag) => tag !== name)
  } else {
    selected.value = [...selected.value, name]
  }
}
</script>
