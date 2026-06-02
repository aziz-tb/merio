<!-- frontend/apps/main/src/App.vue -->
<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import { useStorage } from '@vueuse/core'
import { RouterView } from 'vue-router'
import { useUserStore } from './stores/user'
import { initWS } from './websocket.js'
import { EMITTER_EVENTS } from './constants/emitterEvents.js'
import { useEmitter } from './composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useConversationStore } from './stores/conversation'
import { CONVERSATION_LIST_TYPE } from './constants/conversation'
import { useInboxStore } from './stores/inbox'
import { useUsersStore } from './stores/users'
import { useTeamStore } from './stores/team'
import { useSlaStore } from './stores/sla'
import { useMacroStore } from './stores/macro'
import { useSharedViewStore } from './stores/sharedView'
import { useTagStore } from './stores/tag'
import { useCustomAttributeStore } from './stores/customAttributes'
import { useIdleDetection } from './composables/useIdleDetection'
import { useNotificationStore } from './stores/notification'
import { initAudioContext } from '@shared-ui/composables/useNotificationSound'
import ViewForm from '@/features/view/ViewForm.vue'
import { toast as sooner } from 'vue-sonner'
import Sidebar from '@main/components/sidebar/Sidebar.vue'
import IconRail from '@main/components/sidebar/IconRail.vue'
import Command from '@/features/command/CommandBox.vue'
import CreateConversation from '@/features/conversation/CreateConversation.vue'
import SmallScreenOverlay from '@/components/SmallScreenOverlay.vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import RouteContextHost from '@main/components/layout/RouteContextHost.vue'
import { TooltipProvider } from '@shared-ui/components/ui/tooltip'
import api from '@main/api'
import PathPickerPanel from '@/features/admin/detection/PathPickerPanel.vue'
import { useAgentRightRail } from '@/composables/useAgentRightRail'

const route = useRoute()
const emitter = useEmitter()

// Small screen overlay - shown once per session for screens < 768px.
const showSmallScreenOverlay = ref(window.screen.width < 768 && !sessionStorage.getItem('smallScreenDismissed'))
function dismissSmallScreen() {
  sessionStorage.setItem('smallScreenDismissed', '1')
  showSmallScreenOverlay.value = false
}

// True for any /inboxes/* route. Used to drop the outer card on inbox routes
// where InboxLayout's two ResizablePanels supply their own card frames.
const isInboxRoute = computed(() => route.path.startsWith('/inboxes'))

// True for routes that render their own multi-panel card frames and
// must NOT be wrapped in App.vue's outer rounded card.
// Inbox qualifies; the agent-edit three-rail layout opts in via
// meta.fullBleed. Keep this distinct from isInboxRoute — the
// padding-start logic (ps-[var(--page-space)]) is inbox-specific
// because only the inbox sits flush against the secondary sidebar.
const isFullBleedRoute = computed(
  () => isInboxRoute.value || route.matched.some((r) => r.meta?.fullBleed)
)
const isAgentEditRoute = computed(() =>
  route.matched.some((record) => record.meta?.layoutContext === 'ai-agent-edit')
)
const rail = useAgentRightRail()
const showGlobalPathPicker = computed(() => rail.isActive('path-picker') && !isAgentEditRoute.value)

// Open/closed state for the secondary sidebar — shared with Sidebar.vue and PageHeader.vue.
// When closed, the sidebar slides off-canvas and main content reclaims the 240px.
const { open: sidebarOpen } = useLayoutSidebar()

// Remember last inbox path so navigating back from admin/contacts/reports restores it
const lastInboxPath = useStorage('lastInboxPath', '')
watch(
  () => route.path,
  (path) => {
    if (path.startsWith('/inboxes') && path !== '/inboxes/search') {
      lastInboxPath.value = path.replace(/\/conversation\/[^/]+$/, '')
    }
  }
)
const userStore = useUserStore()
const conversationStore = useConversationStore()
const usersStore = useUsersStore()
const teamStore = useTeamStore()
const inboxStore = useInboxStore()
const slaStore = useSlaStore()
const macroStore = useMacroStore()
const sharedViewStore = useSharedViewStore()
const tagStore = useTagStore()
const customAttributeStore = useCustomAttributeStore()
const userViews = ref([])
const view = ref({})
const openCreateViewForm = ref(false)
const openCreateConversationDialog = ref(false)
const { t } = useI18n()
const notificationStore = useNotificationStore()

// Update browser tab title with unread notification count.
// Watch both unreadCount and route so the prefix is preserved after navigation.
watch([() => notificationStore.unreadCount, () => route.fullPath], ([count]) => {
  const base = document.title.replace(/^\(\d+\)\s*/, '')
  document.title = count > 0 ? `(${count}) ${base}` : base
})

initWS()
useIdleDetection()

// Unlock audio on first user interaction (browser autoplay policy)
const unlockAudio = () => {
  initAudioContext()
  document.removeEventListener('click', unlockAudio)
  document.removeEventListener('touchstart', unlockAudio)
}
document.addEventListener('click', unlockAudio)
document.addEventListener('touchstart', unlockAudio)

onMounted(() => {
  initToaster()
  listenViewRefresh()
  initStores()
})

// Initialize data stores
const initStores = async () => {
  if (!userStore.userID) {
    await userStore.getCurrentUser()
  }
  await Promise.allSettled([
    getUserViews(),
    sharedViewStore.loadSharedViews(),
    conversationStore.fetchStatuses(),
    conversationStore.fetchPriorities(),
    conversationStore.fetchAllDrafts(),
    usersStore.fetchUsers(),
    teamStore.fetchTeams(),
    inboxStore.fetchInboxes(),
    slaStore.fetchSlas(),
    macroStore.loadMacros(),
    tagStore.fetchTags(),
    customAttributeStore.fetchCustomAttributes()
  ])
}

const editView = (v) => {
  view.value = { ...v }
  openCreateViewForm.value = true
}

const deleteView = async (view) => {
  try {
    await api.deleteView(view.id)
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'view' })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.deletedSuccessfully')
    })
  } catch (err) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(err).message
    })
  }
}

const getUserViews = async () => {
  try {
    const response = await api.getCurrentUserViews()
    userViews.value = response.data.data
  } catch (err) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(err).message
    })
  }
}

const initToaster = () => {
  emitter.on(EMITTER_EVENTS.SHOW_TOAST, (message) => {
    if (message.variant === 'destructive') {
      sooner.error(message.description)
    } else if (message.variant === 'warning') {
      sooner.warning(message.description)
    } else {
      sooner.success(message.description)
    }
  })
}

const listenViewRefresh = () => {
  emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshViews)
}

const refreshViews = async (data) => {
  openCreateViewForm.value = false
  // TODO: move model to constants.
  if (data?.model === 'view') {
    await getUserViews()
    const openID = route.params.viewID
    // If the open view was edited its filters may have changed, refetch.
    if (openID && userViews.value.some((v) => String(v.id) === String(openID))) {
      // Reset list and fetch conversations.
      conversationStore.resetConversations()
      conversationStore.fetchConversationsList(true, CONVERSATION_LIST_TYPE.VIEW, 0, [], openID)
    }
  }
}
</script>

<template>
  <SmallScreenOverlay v-if="showSmallScreenOverlay" @dismiss="dismissSmallScreen" />

  <TooltipProvider :delay-duration="0">
    <RouteContextHost>
      <div class="bg-canvas h-screen text-foreground">
        <IconRail />

        <Sidebar
          :userTeams="userStore.teams"
          :userViews="userViews"
          :sharedViews="sharedViewStore.sharedViewList"
          @create-view="openCreateViewForm = true"
          @edit-view="editView"
          @delete-view="deleteView"
          @create-conversation="() => (openCreateConversationDialog = true)"
        />

        <!-- Main content area, offset by both fixed columns. Margin shrinks when the secondary sidebar slides out.
             The start-padding is dropped only when on an inbox route AND the sidebar is open — that's the only
             case where we want the conversation list to sit flush against the sidebar with no canvas gap. In every
             other case (non-inbox route, or sidebar collapsed) we add ps so panels read as standalone rounded cards. -->
        <div
          :class="[
            'h-screen py-[var(--page-space)] pe-[var(--page-space)] overflow-hidden transition-[margin-inline-start] duration-200 ease-out',
            (isInboxRoute && sidebarOpen) ? '' : 'ps-[var(--page-space)]',
            sidebarOpen
              ? 'ms-[calc(var(--ld-sidebar-collapsed-width)+var(--ld-sidebar-width))]'
              : 'ms-[var(--ld-sidebar-collapsed-width)]'
          ]"
        >
          <div class="flex h-full gap-1">
            <div
              :class="[
                'flex flex-col h-full overflow-hidden min-w-0',
                showGlobalPathPicker ? 'flex-1' : 'w-full',
                isFullBleedRoute
                  ? ''
                  : 'rounded-lg bg-background border border-input shadow-sm'
              ]"
            >
              <RouterView class="flex-grow" />
            </div>
            <div
              v-if="showGlobalPathPicker"
              class="h-full w-[min(420px,35vw)] shrink-0 bg-background border border-input shadow-sm overflow-hidden rounded-lg"
            >
              <PathPickerPanel />
            </div>
          </div>
          <ViewForm v-model:openDialog="openCreateViewForm" v-model:view="view" />
        </div>
      </div>
    </RouteContextHost>
  </TooltipProvider>

  <!-- Command box -->
  <Command />

  <!-- Create conversation dialog -->
  <CreateConversation v-model="openCreateConversationDialog" v-if="openCreateConversationDialog" />
</template>
