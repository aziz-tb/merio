<!-- frontend/apps/main/src/components/editor/MacroSuggestionList.vue -->
<template>
  <div
    class="macro-suggestion-list flex flex-col overflow-hidden rounded-lg border border-border bg-background shadow-md"
    :style="{ width: '40rem', height: '18.7rem' }"
  >
    <div class="grid grid-cols-[18rem_1fr] flex-1 min-h-0">
      <div class="relative p-3 overflow-y-auto min-h-0 after:absolute after:right-0 after:top-3 after:bottom-3 after:w-px after:bg-border">
        <div class="mb-2 text-xs font-medium uppercase text-muted-foreground">
          {{ t('macro.heading') }}
        </div>
        <div v-if="items.length > 0" class="space-y-1">
          <button
            v-for="(item, index) in items"
            :key="item.id"
            class="macro-suggestion-item w-full rounded px-2 py-2 text-left text-sm transition-colors hover:bg-muted/60"
            :class="{ 'bg-muted/60': index === selectedIndex }"
            @mouseenter="selectedIndex = index"
            @click="selectItem(index)"
          >
            <span class="block truncate font-medium text-foreground">
              {{ item.name }}
            </span>
          </button>
        </div>
        <div v-else class="py-8 text-center text-sm text-muted-foreground">
          {{ t('macro.noResults') }}
        </div>
      </div>

      <div class="p-3 overflow-y-auto min-h-0">
        <div v-if="selectedMacro" class="space-y-3">
          <h4 class="text-lg font-semibold text-foreground truncate">
            {{ selectedMacro.name }}
          </h4>
          <div
            v-if="selectedMacro.message_content"
            class="native-html text-sm text-foreground break-words border-b border-dashed border-gray-300 pb-3 overflow-y-auto min-h-0 flex-1"
            v-html="selectedMacro.message_content"
          />
          <MacroActionsPreview
            v-if="selectedMacro.actions?.length > 0"
            :actions="selectedMacro.actions"
            readonly
          />
        </div>
        <div v-else class="flex h-full items-center justify-center text-sm text-muted-foreground">
          {{ t('macro.previewHint') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MacroActionsPreview from '@main/features/conversation/MacroActionsPreview.vue'

const { t } = useI18n()

const props = defineProps({
  items: {
    type: Array,
    default: () => []
  },
  command: {
    type: Function,
    required: true
  }
})

const selectedIndex = ref(0)
const selectedMacro = computed(() => props.items[selectedIndex.value])

const selectItem = (index) => {
  const item = props.items[index]
  if (item) {
    props.command(item)
  }
}

const upHandler = () => {
  if (!props.items.length) return
  selectedIndex.value = (selectedIndex.value + props.items.length - 1) % props.items.length
}

const downHandler = () => {
  if (!props.items.length) return
  selectedIndex.value = (selectedIndex.value + 1) % props.items.length
}

const enterHandler = () => {
  selectItem(selectedIndex.value)
}

watch(
  () => props.items,
  () => {
    selectedIndex.value = 0
  }
)

watch(selectedIndex, () => {
  nextTick(() => {
    document.querySelector('.macro-suggestion-item.bg-muted\\/60')?.scrollIntoView({
      block: 'nearest'
    })
  })
})

defineExpose({
  onKeyDown: ({ event }) => {
    if (event.key === 'ArrowUp') {
      upHandler()
      return true
    }
    if (event.key === 'ArrowDown') {
      downHandler()
      return true
    }
    if (event.key === 'Enter') {
      enterHandler()
      return true
    }
    if (event.key === 'Escape') {
      return true
    }
    return false
  }
})
</script>