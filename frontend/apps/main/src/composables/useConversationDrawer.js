import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { INBOX_VIEW_MODES, useInboxView } from '@main/composables/useInboxView'

const conversationRouteToListRoute = {
  'inbox-conversation': 'inbox',
  'team-inbox-conversation': 'team-inbox',
  'view-inbox-conversation': 'view-inbox'
}

export function useConversationDrawer() {
  const route = useRoute()
  const router = useRouter()
  const { isTableView, setViewMode } = useInboxView()

  const isConversationRoute = computed(() =>
    Object.prototype.hasOwnProperty.call(conversationRouteToListRoute, route.name)
  )

  const isConversationDrawerOpen = computed(() =>
    isTableView.value && isConversationRoute.value
  )

  const closeConversationDrawer = () => {
    const listRouteName = conversationRouteToListRoute[route.name]
    if (!listRouteName) return

    router.push({
      name: listRouteName,
      params: {
        ...(listRouteName === 'inbox' && { type: route.params.type || 'assigned' }),
        ...(listRouteName === 'team-inbox' && { teamID: route.params.teamID }),
        ...(listRouteName === 'view-inbox' && { viewID: route.params.viewID })
      }
    })
  }

  const expandConversationDrawer = () => {
    setViewMode(INBOX_VIEW_MODES.COLUMNS)
  }

  return {
    isConversationDrawerOpen,
    closeConversationDrawer,
    expandConversationDrawer
  }
}
