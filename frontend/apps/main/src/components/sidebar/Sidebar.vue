<!-- frontend/apps/main/src/components/sidebar/Sidebar.vue -->
<script setup>
import {
  adminNavItems,
  reportsNavItems,
  accountNavItems,
  contactNavItems
} from '../../constants/navigation'
import { useRoute, useRouter } from 'vue-router'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@shared-ui/components/ui/collapsible'
import {
  SidebarContent,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubItem
} from '@shared-ui/components/ui/sidebar'
import { useAppSettingsStore } from '@main/stores/appSettings'
import { At, BookOpen, Database, ChevronRight, EllipsisVertical, CircleUser, Gear, Plug, CircleDashed, CircleDot, Plus, Inbox as InboxIcon, MagnifyingGlass, LayerGroup, Clock, Bolt, Eye, Webhook, Link, Timer, Key, Shield, Users, User, Scroll, Tags, Sliders, Envelope, FileLines, DiagramNested, ChartBar, AddressCard, UserMagnifyingGlass, Vault} from '@qall/icons'


const navIconMap = {
  Settings: Gear,
  Clock,
  Timer,
  Inbox: InboxIcon,
  CircleDot,
  Tag: Tags,
  SlidersHorizontal: Sliders,
  Eye,
  Zap: Bolt,
  Workflow: DiagramNested,
  UserRound: User,
  UsersRound: Users,
  Shield,
  ScrollText: Scroll,
  Mail: Envelope,
  FileText: FileLines,
  KeyRound: Key,
  Vault,
  Webhook,
  Link,
  Database: UserMagnifyingGlass,
  Plug,
  BarChart3: ChartBar,
  CircleUser,
  Contact: AddressCard,
  BookOpen
}
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { Button } from '@shared-ui/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@shared-ui/components/ui/alert-dialog'
import { filterNavItems } from '@main/utils/nav-permissions'
import { permissions } from '@main/constants/permissions'
import { useStorage } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@main/stores/user'
import { useConversationStore } from '@main/stores/conversation'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import KnowledgeSidebar from '@main/components/sidebar/KnowledgeSidebar.vue'
import AiSidebar from '@main/components/sidebar/AiSidebar.vue'

defineProps({
  userTeams: { type: Array, default: () => [] },
  userViews: { type: Array, default: () => [] },
  sharedViews: { type: Array, default: () => [] }
})
const userStore = useUserStore()
const conversationStore = useConversationStore()
const settingsStore = useAppSettingsStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const emit = defineEmits(['createView', 'editView', 'deleteView', 'createConversation'])

const isActiveParent = (parentHref) => {
  return route.path.startsWith(parentHref)
}

const isInboxRoute = (path) => {
  return path.startsWith('/inboxes')
}

const openCreateViewDialog = () => {
  emit('createView')
}

const editView = (view) => {
  emit('editView', view)
}

const openDeleteConfirmation = (view) => {
  viewToDelete.value = view
  isDeleteOpen.value = true
}

const handleDeleteView = () => {
  if (viewToDelete.value) {
    emit('deleteView', viewToDelete.value)
    isDeleteOpen.value = false
    viewToDelete.value = null
  }
}

// Navigation methods with conversation retention
const navigateToInbox = (type) => {
  if (conversationStore.isConversationOpen && conversationStore.conversation.data?.uuid) {
    router.push({
      name: 'inbox-conversation',
      params: {
        type,
        uuid: conversationStore.conversation.data.uuid
      }
    })
  } else {
    router.push({
      name: 'inbox',
      params: { type }
    })
  }
}

const navigateToTeamInbox = (teamID) => {
  if (conversationStore.isConversationOpen && conversationStore.conversation.data?.uuid) {
    router.push({
      name: 'team-inbox-conversation',
      params: {
        teamID,
        uuid: conversationStore.conversation.data.uuid
      }
    })
  } else {
    router.push({
      name: 'team-inbox',
      params: { teamID }
    })
  }
}

const navigateToViewInbox = (viewID) => {
  if (conversationStore.isConversationOpen && conversationStore.conversation.data?.uuid) {
    router.push({
      name: 'view-inbox-conversation',
      params: {
        viewID,
        uuid: conversationStore.conversation.data.uuid
      }
    })
  } else {
    router.push({
      name: 'view-inbox',
      params: { viewID }
    })
  }
}

const filteredAdminNavItems = computed(() => filterNavItems(adminNavItems, userStore.can))
const filteredReportsNavItems = computed(() => filterNavItems(reportsNavItems, userStore.can))
const filteredContactsNavItems = computed(() => filterNavItems(contactNavItems, userStore.can))

// For auto opening admin collapsibles when a child route is active
const openAdminCollapsible = ref(null)
const toggleAdminCollapsible = (titleKey) => {
  openAdminCollapsible.value = openAdminCollapsible.value === titleKey ? null : titleKey
}
// Watch for route changes and update the active collapsible
watch(
  [() => route.path, filteredAdminNavItems],
  () => {
    const activeItem = filteredAdminNavItems.value.find((item) => {
      if (!item.children) return isActiveParent(item.href)
      return item.children.some((child) => isActiveParent(child.href))
    })
    if (activeItem) {
      openAdminCollapsible.value = activeItem.titleKey
    }
  },
  { immediate: true }
)

// Open/closed state for the whole secondary sidebar (drives the slide-out animation).
const { open: sidebarOpen } = useLayoutSidebar()

// Per-section collapse state for the inbox mode's nested groups.
const teamInboxOpen = useStorage('teamInboxOpen', true)
const viewInboxOpen = useStorage('viewInboxOpen', true)
const sharedViewInboxOpen = useStorage('sharedViewInboxOpen', true)

// Track which view is being hovered for ellipsis menu visibility
const hoveredViewId = ref(null)

// Track delete confirmation dialog state
const isDeleteOpen = ref(false)
const viewToDelete = ref(null)

// Whether ANY inner template in the <aside> would render content for
// the current route. The aside carries bg-sidebar + border + rounded
// styling, so without this guard it shows as an empty card strip on
// routes that opt out of every sidebar mode. Mirror each v-if in the
// template below exactly.
const hasSidebarContent = computed(
  () =>
    route.matched.some((r) => r.name && r.name.startsWith('contact')) ||
    (userStore.hasReportTabPermissions &&
      route.matched.some((r) => r.name && r.name.startsWith('reports'))) ||
    (route.path && route.path.startsWith('/knowledge')) ||
    (route.path && route.path.startsWith('/ai')) ||
    route.matched.some((r) => r.name && r.name.startsWith('admin')) ||
    isActiveParent('/account') ||
    (route.path && isInboxRoute(route.path))
)
</script>

<template>
  <aside
    :class="[
      'fixed z-10 top-[var(--page-space)] bottom-[var(--page-space)] start-[var(--ld-sidebar-collapsed-width)] flex flex-col w-[var(--ld-sidebar-width)] bg-sidebar overflow-hidden transition-transform duration-200 ease-out',
      isInboxRoute(route.path)
        ? 'border-y border-l border-input rounded-l-lg'
        : 'border border-input rounded-lg',
      sidebarOpen ? 'translate-x-0' : '-translate-x-full rtl:translate-x-full'
    ]"
  >
    <!-- Contacts sidebar -->
    <template
      v-if="route.matched.some((record) => record.name && record.name.startsWith('contact'))"
    >
      <div class="flex flex-col h-full">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <div class="px-1">
                <span class="text-xs font-normal text-muted-foreground uppercase tracking-wider px-2">
                  {{ t('globals.terms.contact', 2) }}
                </span>
              </div>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem v-for="item in filteredContactsNavItems" :key="item.titleKey">
                <SidebarMenuButton :isActive="isActiveParent(item.href)" asChild>
                  <router-link :to="item.href">
                    <component :is="navIconMap[item.icon]" v-if="item.icon" />
                    <span>{{ t(item.allLabelKey) }}</span>
                  </router-link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
      </div>
    </template>

    <!-- Reports sidebar -->
    <template
      v-if="
        userStore.hasReportTabPermissions &&
        route.matched.some((record) => record.name && record.name.startsWith('reports'))
      "
    >
      <div class="flex flex-col h-full">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <div class="px-1">
                <span class="text-xs font-normal text-muted-foreground uppercase tracking-wider px-2">
                  {{ t('globals.terms.report', 2) }}
                </span>
              </div>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem v-for="item in filteredReportsNavItems" :key="item.titleKey">
                <SidebarMenuButton :isActive="isActiveParent(item.href)" asChild>
                  <router-link :to="item.href">
                    <component :is="navIconMap[item.icon]" v-if="item.icon" />
                    <span>{{ t(item.titleKey) }}</span>
                  </router-link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
      </div>
    </template>

    <!-- Knowledge sidebar -->
    <template v-if="route.path && route.path.startsWith('/knowledge')">
      <KnowledgeSidebar />
    </template>

    <!-- AI sidebar -->
    <template v-if="route.path && route.path.startsWith('/ai')">
      <AiSidebar />
    </template>

    <!-- Admin Sidebar -->
    <template v-if="route.matched.some((record) => record.name && record.name.startsWith('admin'))">
      <div class="flex flex-col h-full">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <div class="flex flex-col items-start justify-between w-full px-1">
                <span class="text-sm font-semibold text-muted-foreground uppercase tracking-wider px-2">
                  {{ t('globals.terms.admin') }}
                </span>
                <!-- App version -->
                <div class="text-xs text-muted-foreground">
                  ({{ settingsStore.settings['app.version'] }})
                </div>
              </div>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem v-for="item in filteredAdminNavItems" :key="item.titleKey">
                <SidebarMenuButton
                  v-if="!item.children"
                  :isActive="isActiveParent(item.href)"
                  asChild
                >
                  <router-link :to="item.href">
                    <span>{{ t(item.titleKey) }}</span>
                  </router-link>
                </SidebarMenuButton>

                <Collapsible
                  v-else
                  class="group/collapsible"
                  :open="openAdminCollapsible === item.titleKey"
                  @update:open="toggleAdminCollapsible(item.titleKey)"
                >
                  <CollapsibleTrigger as-child>
                    <SidebarMenuButton :isActive="isActiveParent(item.href)">
                      <span>{{ t(item.titleKey, item.isTitleKeyPlural === true ? 2 : 1) }}</span>
                      <ChevronRight
                        class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                      />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <SidebarMenuSub>
                      <SidebarMenuSubItem v-for="child in item.children" :key="child.titleKey">
                        <SidebarMenuButton size="sm" :isActive="isActiveParent(child.href)" asChild>
                          <router-link :to="child.href">
                            <component :is="navIconMap[child.icon]" v-if="child.icon" />
                            <span>{{ t(child.titleKey, child.isTitleKeyPlural === true ? 2 : 1) }}</span>
                          </router-link>
                        </SidebarMenuButton>
                      </SidebarMenuSubItem>
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </Collapsible>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
      </div>
    </template>

    <!-- Account sidebar -->
    <template v-if="isActiveParent('/account')">
      <div class="flex flex-col h-full">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <div class="px-1">
                <span class="text-xs font-normal text-muted-foreground uppercase tracking-wider px-2">
                  {{ t('globals.terms.account') }}
                </span>
              </div>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem v-for="item in accountNavItems" :key="item.titleKey">
                <SidebarMenuButton :isActive="isActiveParent(item.href)" asChild>
                  <router-link :to="item.href">
                    <component :is="navIconMap[item.icon]" v-if="item.icon" />
                    <span>{{ t(item.titleKey) }}</span>
                  </router-link>
                </SidebarMenuButton>
                <SidebarMenuAction>
                  <span class="sr-only">{{ item.description }}</span>
                </SidebarMenuAction>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
      </div>
    </template>

    <!-- Inbox sidebar -->
    <template v-if="route.path && isInboxRoute(route.path)">
      <div class="flex flex-col h-full">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <div class="flex items-center justify-between w-full">
                <span class="text-lg font-semibold text-foreground pl-2">{{ t('globals.terms.inbox') }}</span>
                <div class="flex items-center gap-1">
                  <Button asChild variant="ghost" size="icon" class="size-7 text-muted-foreground hover:text-foreground">
                    <router-link :to="{ name: 'search' }" :title="t('globals.terms.search')">
                      <MagnifyingGlass class="size-4" />
                    </router-link>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="size-7 text-muted-foreground hover:text-foreground"
                    :title="t('globals.messages.new', { name: t('globals.terms.conversation') })"
                    @click="emit('createConversation')"
                  >
                    <Plus class="size-4" />
                  </Button>
                </div>
              </div>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton :isActive="isActiveParent('/inboxes/assigned')" @click="navigateToInbox('assigned')">
                    <InboxIcon />
                    <span>{{ t('globals.terms.myInbox') }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>

              <SidebarMenuItem>
                <SidebarMenuButton :isActive="isActiveParent('/inboxes/mentioned')" @click="navigateToInbox('mentioned')">
                    <At />
                    <span>
                      {{ t('globals.terms.mention', 2) }}
                    </span>
                </SidebarMenuButton>
              </SidebarMenuItem>

              <SidebarMenuItem>
                <SidebarMenuButton :isActive="isActiveParent('/inboxes/unassigned')" @click="navigateToInbox('unassigned')">
                    <CircleDashed />
                    <span>
                      {{ t('globals.terms.unassigned') }}
                    </span>
                </SidebarMenuButton>
              </SidebarMenuItem>

              <SidebarMenuItem>
                <SidebarMenuButton :isActive="isActiveParent('/inboxes/all')" @click="navigateToInbox('all')">
                    <LayerGroup />
                    <span>
                      {{ t('globals.messages.all') }}
                    </span>
                </SidebarMenuButton>
              </SidebarMenuItem>

              <!-- Team Inboxes -->
              <Collapsible
                defaultOpen
                class="group/collapsible"
                v-if="userTeams.length"
                v-model:open="teamInboxOpen"
              >
                <SidebarMenuItem>
                  <CollapsibleTrigger as-child>
                    <SidebarMenuButton>
                        <span>
                          {{ t('globals.terms.teamInbox', 2) }}
                        </span>
                        <ChevronRight
                          class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                        />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <SidebarMenuSub>
                      <SidebarMenuSubItem v-for="team in userTeams" :key="team.id">
                        <SidebarMenuButton
                          size="sm"
                          :is-active="route.params.teamID == team.id"
                          @click="navigateToTeamInbox(team.id)"
                        >
                          {{ team.emoji }}<span>{{ team.name }}</span>
                        </SidebarMenuButton>
                      </SidebarMenuSubItem>
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>

              <!-- Views -->
              <Collapsible class="group/collapsible" defaultOpen v-model:open="viewInboxOpen" v-if="userStore.can(permissions.VIEW_MANAGE)">
                <SidebarMenuItem>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuButton class="group/item !p-2">
                        <span>
                          {{ t('globals.terms.view', 2) }}
                        </span>
                        <div>
                          <Plus
                            size="18"
                            @click.stop="openCreateViewDialog"
                            class="rounded cursor-pointer opacity-0 transition-colors duration-200 group-hover/item:opacity-100 hover:bg-sidebar-accent text-muted-foreground hover:text-sidebar-accent-foreground p-1"
                          />
                        </div>
                        <ChevronRight
                          class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                          v-if="userViews.length"
                        />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>

                  <CollapsibleContent>
                    <SidebarMenuSub>
                      <SidebarMenuSubItem
                        v-for="view in userViews" :key="view.id"
                        @mouseenter="hoveredViewId = view.id"
                        @mouseleave="hoveredViewId = null"
                      >
                        <SidebarMenuButton
                          size="sm"
                          :isActive="route.params.viewID == view.id"
                          @click="navigateToViewInbox(view.id)"
                        >
                          <span class="flex-1 truncate" :title="view.name">{{ view.name }}</span>
                        </SidebarMenuButton>
                        <SidebarMenuAction
                          :class="[
                            'mr-3',
                            'md:opacity-0',
                            'data-[state=open]:opacity-100',
                            { 'md:opacity-100': hoveredViewId === view.id }
                          ]"
                        >
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild @click.prevent>
                              <EllipsisVertical />
                            </DropdownMenuTrigger>
                            <DropdownMenuContent>
                              <DropdownMenuItem @click="() => editView(view)">
                                <span>{{ t('globals.messages.edit') }}</span>
                              </DropdownMenuItem>
                              <DropdownMenuItem @click="() => openDeleteConfirmation(view)">
                                <span>{{ t('globals.messages.delete') }}</span>
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </SidebarMenuAction>
                      </SidebarMenuSubItem>
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>

              <!-- Shared Views -->
              <Collapsible
                class="group/collapsible"
                defaultOpen
                v-model:open="sharedViewInboxOpen"
                v-if="sharedViews.length"
              >
                <SidebarMenuItem>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuButton class="!p-2">
                        <span>
                          {{ t('globals.terms.sharedView', 2) }}
                        </span>
                        <ChevronRight
                          class="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                        />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>

                  <CollapsibleContent>
                    <SidebarMenuSub>
                      <SidebarMenuSubItem v-for="view in sharedViews" :key="view.id">
                        <SidebarMenuButton
                          size="sm"
                          :isActive="route.params.viewID == view.id"
                          @click="navigateToViewInbox(view.id)"
                        >
                          <span class="flex-1 truncate" :title="view.name">{{
                            view.name
                          }}</span>
                        </SidebarMenuButton>
                      </SidebarMenuSubItem>
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>
      </div>
    </template>
  </aside>

  <!-- View Delete Confirmation Dialog -->
  <AlertDialog v-model:open="isDeleteOpen">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ t('confirm.deleteView') }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="handleDeleteView">
          {{ t('globals.messages.delete') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
