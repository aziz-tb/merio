<template>
  <div class="relative h-full">
    <ResizablePanelGroup
      v-if="showContent"
      direction="horizontal"
      class="h-full"
      @layout="onLayoutChange"
    >
      <!-- Conversation Content Panel — uses bg-sidebar (warm off-white) instead of pure white for a softer thread surface. -->
      <ResizablePanel :default-size="sidebarOpen ? panelSizes[0] : 100" :min-size="50">
        <div
          :class="[
            'h-full bg-sidebar border border-input rounded-lg shadow-sm overflow-hidden',
            sidebarOpen ? 'mr-0.5' : ''
          ]"
        >
          <Conversation />
        </div>
      </ResizablePanel>

      <!-- Resizable Handle -->
      <ResizableHandle class="opacity-0" />

      <!-- Sidebar Panel (collapsible) -->
      <ResizablePanel
        ref="sidebarPanelRef"
        :default-size="panelSizes[1]"
        :min-size="22"
        :max-size="40"
        :collapsible="true"
        :collapsed-size="0"
        @collapse="onSidebarCollapse"
        @expand="onSidebarExpand"
      >
        <div class="h-full bg-background border border-input rounded-lg shadow-sm overflow-hidden ml-0.5">
          <div class="h-full overflow-y-auto overflow-x-hidden">
            <ConversationSideBar />
          </div>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useStorage, useDocumentVisibility } from '@vueuse/core'
import { useConversationStore } from '@main/stores/conversation'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import Conversation from '@main/features/conversation/Conversation.vue'
import ConversationSideBar from '@main/features/conversation/sidebar/ConversationSideBar.vue'
import { ResizablePanelGroup, ResizablePanel, ResizableHandle } from '@shared-ui/components/ui/resizable'

const props = defineProps({
  uuid: String
})

const conversationStore = useConversationStore()
const emitter = useEmitter()
const sidebarPanelRef = ref(null)
const sidebarOpen = useStorage('conversationSidebarOpen', true)
const panelSizes = useStorage('conversationDetailPanelSizes', [70, 30])

const showContent = computed(
  () => conversationStore.current || conversationStore.conversation.loading
)

const toggleSidebar = () => {
  if (sidebarOpen.value) {
    sidebarPanelRef.value?.collapse()
  } else {
    sidebarPanelRef.value?.expand()
  }
}

const onSidebarCollapse = () => {
  sidebarOpen.value = false
}

const onSidebarExpand = () => {
  sidebarOpen.value = true
}

const onLayoutChange = (sizes) => {
  if (sidebarOpen.value && sizes.length === 2) {
    panelSizes.value = sizes
  }
}

// Listen to emitter events for toggle (from sidebar contact)
onMounted(() => {
  emitter.on(EMITTER_EVENTS.CONVERSATION_SIDEBAR_TOGGLE, toggleSidebar)

  // Sync initial collapsed state from localStorage
  nextTick(() => {
    if (!sidebarOpen.value && sidebarPanelRef.value) {
      sidebarPanelRef.value.collapse()
    }
  })
})

// React to external sidebarOpen changes (e.g. floating layout toggle in ConversationList).
watch(sidebarOpen, (open) => {
  if (!sidebarPanelRef.value) return
  if (open) sidebarPanelRef.value.expand()
  else sidebarPanelRef.value.collapse()
})

const visibility = useDocumentVisibility()
watch(visibility, (state) => {
  if (state === 'visible' && props.uuid) {
    conversationStore.updateAssigneeLastSeen(props.uuid)
  }
})

onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.CONVERSATION_SIDEBAR_TOGGLE, toggleSidebar)
})

const fetchConversation = async (uuid) => {
  await Promise.all([
    conversationStore.fetchConversation(uuid),
    conversationStore.fetchMessages(uuid),
    conversationStore.fetchParticipants(uuid)
  ])
  await conversationStore.updateAssigneeLastSeen(uuid)
}

// Initial fetch
onMounted(() => {
  if (props.uuid) fetchConversation(props.uuid)
})

// Watcher for UUID changes
watch(
  () => props.uuid,
  (newUUID, oldUUID) => {
    if (newUUID && newUUID !== oldUUID) {
      fetchConversation(newUUID)
    }
  }
)
</script>
