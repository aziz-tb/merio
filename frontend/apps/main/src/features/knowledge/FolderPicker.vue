<template>
  <Popover v-model:open="popoverOpen">
    <PopoverTrigger as-child>
      <Button type="button" variant="outline" class="w-full justify-between">
        <span class="truncate">{{ displayLabel }}</span>
        <ChevronsUpDown class="size-4 opacity-50" />
      </Button>
    </PopoverTrigger>
    <PopoverContent class="p-0 w-[300px] max-h-[400px] overflow-auto">
      <div class="p-2">
        <div
          v-if="allowNull"
          :class="['cursor-pointer rounded px-2 py-1 hover:bg-muted', modelValue === null && 'bg-muted font-medium']"
          @click="select(null)"
        >
          {{ t('admin.knowledge.folder.root') }}
        </div>
        <FolderTreeNode
          v-for="root in roots"
          :key="root.id"
          :node="root"
          :children-by-parent="childrenByParent"
          :selected-id="modelValue"
          :depth="0"
          @select="select"
          @new-subfolder="() => {}"
          @rename="() => {}"
          @move="() => {}"
          @delete="() => {}"
        />
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronsUpDown } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button/index.js'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@shared-ui/components/ui/popover/index.js'
import FolderTreeNode from './FolderTreeNode.vue'

const { t } = useI18n()

const props = defineProps({
  modelValue: { type: [Number, null], default: null },
  folders: { type: Array, default: () => [] },
  allowNull: { type: Boolean, default: false },
  excludeIds: { type: Array, default: () => [] } // for Move dialog: hide self+descendants
})
const emit = defineEmits(['update:modelValue'])

const popoverOpen = ref(false)

const filtered = computed(() => {
  const exclude = new Set(props.excludeIds)
  return props.folders.filter((f) => !exclude.has(f.id))
})

const childrenByParent = computed(() => {
  const m = new Map()
  for (const f of filtered.value) {
    const k = f.parent_id ?? null
    if (!m.has(k)) m.set(k, [])
    m.get(k).push(f)
  }
  return m
})

const roots = computed(() => childrenByParent.value.get(null) || [])

const displayLabel = computed(() => {
  if (props.modelValue === null) {
    return props.allowNull ? t('admin.knowledge.folder.root') : t('admin.knowledge.folder.placeholder')
  }
  const found = props.folders.find((f) => f.id === props.modelValue)
  return found ? found.name : t('admin.knowledge.folder.placeholder')
})

const select = (id) => {
  emit('update:modelValue', id)
  popoverOpen.value = false
}
</script>
