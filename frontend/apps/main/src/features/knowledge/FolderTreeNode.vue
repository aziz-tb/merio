<template>
  <div>
    <div
      :class="[
        'flex items-center group rounded px-1 py-1 cursor-pointer hover:bg-muted',
        selectedId === node.id && 'bg-muted font-medium'
      ]"
      :style="{ paddingLeft: depth * 12 + 4 + 'px' }"
      @click.stop="$emit('select', node.id)"
    >
      <button
        v-if="children.length"
        type="button"
        class="size-4 flex items-center justify-center text-muted-foreground hover:text-foreground"
        @click.stop="open = !open"
      >
        <ChevronRight :class="['size-3 transition-transform', open && 'rotate-90']" />
      </button>
      <span v-else class="size-4" />
      <Folder class="size-3 mx-1 text-muted-foreground" />
      <span class="flex-1 truncate" :title="node.name">{{ node.name }}</span>
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <button
            type="button"
            class="opacity-0 group-hover:opacity-100 data-[state=open]:opacity-100 size-5 flex items-center justify-center text-muted-foreground hover:text-foreground"
            @click.stop
          >
            <EllipsisVertical class="size-3" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem @click="$emit('new-subfolder', node.id)">
            {{ t('admin.knowledge.actions.newSubfolder') }}
          </DropdownMenuItem>
          <DropdownMenuItem @click="$emit('rename', node)">
            {{ t('admin.knowledge.actions.rename') }}
          </DropdownMenuItem>
          <DropdownMenuItem @click="$emit('move', node)">
            {{ t('admin.knowledge.actions.move') }}
          </DropdownMenuItem>
          <DropdownMenuItem @click="$emit('delete', node)">
            {{ t('globals.messages.delete') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <div v-if="open">
      <FolderTreeNode
        v-for="child in children"
        :key="child.id"
        :node="child"
        :children-by-parent="childrenByParent"
        :selected-id="selectedId"
        :depth="depth + 1"
        @select="(id) => $emit('select', id)"
        @new-subfolder="(parentId) => $emit('new-subfolder', parentId)"
        @rename="(f) => $emit('rename', f)"
        @move="(f) => $emit('move', f)"
        @delete="(f) => $emit('delete', f)"
      />
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Folder, ChevronRight, EllipsisVertical } from "@qall/icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu/index.js'

const { t } = useI18n()

const props = defineProps({
  node: { type: Object, required: true },
  childrenByParent: { type: Map, required: true },
  selectedId: { type: [Number, null], default: null },
  depth: { type: Number, default: 0 }
})

defineEmits(['select', 'new-subfolder', 'rename', 'move', 'delete'])

const open = ref(true)
const children = computed(() => props.childrenByParent.get(props.node.id) || [])
</script>
