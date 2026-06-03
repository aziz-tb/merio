<template>
  <div class="text-sm">
    <!-- Optional header: "Folders" label + add-root button.
         The new KnowledgeList layout supplies its own "All folders" entry
         and bottom-anchored "+ New folder" button, so it sets hideHeader. -->
    <div
      v-if="!hideHeader"
      class="flex items-center justify-between px-2 py-1 border-b border-border mb-1 sticky top-0 bg-background"
    >
      <span class="font-medium">{{ t('globals.terms.folder', 2) }}</span>
      <Button
        variant="ghost"
        size="icon"
        class="size-6"
        :title="t('admin.knowledge.actions.newFolder')"
        @click="$emit('new-root')"
      >
        <Plus class="size-4" />
      </Button>
    </div>
    <div
      v-if="!hideHeader"
      :class="['cursor-pointer rounded px-2 py-1 hover:bg-muted', selectedId === null && 'bg-muted font-medium']"
      @click="$emit('select', null)"
    >
      {{ t('admin.knowledge.allFolders') }}
    </div>
    <FolderTreeNode
      v-for="root in roots"
      :key="root.id"
      :node="root"
      :children-by-parent="childrenByParent"
      :selected-id="selectedId"
      :depth="0"
      @select="(id) => $emit('select', id)"
      @new-subfolder="(parentId) => $emit('new-subfolder', parentId)"
      @rename="(folder) => $emit('rename', folder)"
      @move="(folder) => $emit('move', folder)"
      @delete="(folder) => $emit('delete', folder)"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button/index.js'
import FolderTreeNode from './FolderTreeNode.vue'

const { t } = useI18n()

const props = defineProps({
  folders: { type: Array, default: () => [] },
  selectedId: { type: [Number, null], default: null },
  hideHeader: { type: Boolean, default: false }
})

defineEmits(['select', 'new-subfolder', 'rename', 'move', 'delete', 'new-root'])

const sorted = computed(() =>
  [...props.folders].sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
)

const childrenByParent = computed(() => {
  const m = new Map()
  for (const f of sorted.value) {
    const k = f.parent_id ?? null
    if (!m.has(k)) m.set(k, [])
    m.get(k).push(f)
  }
  return m
})

const roots = computed(() => childrenByParent.value.get(null) || [])
</script>
