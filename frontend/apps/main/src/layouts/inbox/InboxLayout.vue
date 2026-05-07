<!-- frontend/apps/main/src/layouts/inbox/InboxLayout.vue -->
<template>
  <ResizablePanelGroup
    v-if="!isSearchRoute"
    direction="horizontal"
    class="h-full w-full"
    @layout="onLayoutChange"
  >
    <!-- Conversation List Panel — same bg as sidebar.
         When the secondary sidebar is open it sits flush against it (rounded only on the right, no left border).
         When the sidebar is collapsed, the list becomes a standalone rounded card with full border. -->
    <ResizablePanel
      :default-size="panelSizes[0]"
      :min-size="18"
      :max-size="30"
    >
      <div
        :class="[
          'h-full bg-sidebar border border-input shadow-sm overflow-hidden mr-0.5',
          sidebarOpen ? 'rounded-r-xl' : 'rounded-xl'
        ]"
      >
        <ConversationList />
      </div>
    </ResizablePanel>

    <ResizableHandle class="opacity-0" />

    <!-- Conversation Detail wrapper — transparent; the thread + details cards live inside ConversationDetailView. -->
    <ResizablePanel :default-size="panelSizes[1]" :min-size="65">
      <div class="h-full overflow-hidden ml-0.5">
        <router-view v-slot="{ Component }">
          <component :is="Component" />
        </router-view>
      </div>
    </ResizablePanel>
  </ResizablePanelGroup>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useStorage } from '@vueuse/core'
import ConversationList from '@/features/conversation/list/ConversationList.vue'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle
} from '@shared-ui/components/ui/resizable'

const route = useRoute()
const isSearchRoute = computed(() => route.name === 'search')
const { open: sidebarOpen } = useLayoutSidebar()

// Persist panel sizes: [conversationList, conversationDetail]
const panelSizes = useStorage('inboxPanelSizes', [23, 77])

const onLayoutChange = (sizes) => {
  panelSizes.value = sizes
}
</script>
