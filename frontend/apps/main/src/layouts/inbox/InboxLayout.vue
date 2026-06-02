<!-- frontend/apps/main/src/layouts/inbox/InboxLayout.vue -->
<template>
  <div v-if="isTableView && !isSearchRoute" class="h-full w-full transition-all duration-200 ease-out">
    <div class="h-full ml-1 bg-background border border-input shadow-sm overflow-hidden rounded-lg transition-all duration-200 ease-out">
      <ConversationList />
    </div>

    <Sheet :open="drawerOpen" @update:open="handleDrawerOpenChange">
      <SheetContent
        side="right"
        class="inset-y-2 right-2 flex h-auto w-[60vw] max-w-[1200px] flex-col gap-0 overflow-hidden rounded-lg border border-input bg-background p-0 shadow-lg sm:max-w-[1200px] [&>button:last-child]:hidden"
      >
        <SheetTitle class="sr-only">{{ t('globals.terms.conversation', 1) }}</SheetTitle>
        <SheetDescription class="sr-only">{{ t('globals.terms.conversationDescription', 1) }}</SheetDescription>
        <div v-if="drawerOpen" class="min-h-0 flex-1 overflow-hidden bg-background p-2">
          <router-view v-slot="{ Component }">
            <component :is="Component" />
          </router-view>
        </div>
      </SheetContent>
    </Sheet>
  </div>

  <ResizablePanelGroup
    v-else-if="!isSearchRoute"
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
          sidebarOpen ? 'rounded-r-lg' : 'rounded-lg'
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
import { computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useStorage } from '@vueuse/core'
import ConversationList from '@/features/conversation/list/ConversationList.vue'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import { useInboxView } from '@main/composables/useInboxView'
import { useConversationDrawer } from '@main/composables/useConversationDrawer'
import { useConversationStore } from '@main/stores/conversation'
import { CONVERSATION_DEFAULT_STATUSES, CONVERSATION_LIST_TYPE } from '@main/constants/conversation'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle
} from '@shared-ui/components/ui/resizable'
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '@shared-ui/components/ui/sheet'

const route = useRoute()
const { t } = useI18n()
const isSearchRoute = computed(() => route.name === 'search')
const type = computed(() => route.params.type)
const teamID = computed(() => route.params.teamID)
const viewID = computed(() => route.params.viewID)
const { open: sidebarOpen } = useLayoutSidebar()
const { isTableView } = useInboxView()
const {
  isConversationDrawerOpen,
  closeConversationDrawer
} = useConversationDrawer()
const conversationStore = useConversationStore()

// Persist panel sizes: [conversationList, conversationDetail]
const panelSizes = useStorage('inboxPanelSizes', [23, 77])

const drawerOpen = computed(() => isConversationDrawerOpen.value && !isSearchRoute.value)

const onLayoutChange = (sizes) => {
  panelSizes.value = sizes
}

const handleDrawerOpenChange = (open) => {
  if (!open && drawerOpen.value) closeConversationDrawer()
}

const ensureOpenListStatus = () => {
  if (!conversationStore.getListStatus) {
    conversationStore.setListStatus(CONVERSATION_DEFAULT_STATUSES.OPEN, false)
  }
}

const fetchConversationsForRoute = () => {
  if (type.value) {
    ensureOpenListStatus()
    conversationStore.fetchConversationsList(true, type.value)
    return
  }

  if (teamID.value) {
    ensureOpenListStatus()
    conversationStore.fetchConversationsList(
      true,
      CONVERSATION_LIST_TYPE.TEAM_UNASSIGNED,
      teamID.value
    )
    return
  }

  if (viewID.value) {
    conversationStore.setListStatus('', false)
    conversationStore.fetchConversationsList(true, CONVERSATION_LIST_TYPE.VIEW, 0, [], viewID.value)
  }
}

onMounted(fetchConversationsForRoute)

watch(
  [type, teamID, viewID],
  ([newType, newTeamID, newViewID], [oldType, oldTeamID, oldViewID]) => {
    if (newType === oldType && newTeamID === oldTeamID && newViewID === oldViewID) return
    fetchConversationsForRoute()
  }
)
</script>
