<template>
  <div class="h-full flex flex-col relative">
    <!-- Header -->
    <div class="flex items-center gap-2 px-3 h-14 border-b border-border shrink-0 bg-sidebar">
      <button
        type="button"
        class="cursor-pointer inline-flex items-center justify-center size-8 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground"
        @click="toggleSidebar"
      >
        <LayoutLeft class="size-4" />
        <span class="sr-only">Toggle sidebar</span>
      </button>
      <span class="text-base font-semibold truncate">{{ title }}</span>
    </div>

    <ConversationTable v-if="isTableView" class="min-h-0 flex-1" />

    <template v-else>
      <!-- Filters -->
      <div class="px-3 h-10 flex justify-between items-center shrink-0">
        <!-- Status dropdown-menu, hidden when a view is selected as views are pre-filtered -->
        <DropdownMenu v-if="!route.params.viewID">
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" class="px-2 text-xs text-muted-foreground hover:text-foreground gap-1">
              <span>{{ conversationStore.conversations.total }}</span>
              <span>{{ listStatusLabel }}</span>
              <ChevronDown class="size-3 opacity-50" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem
              v-for="status in conversationStore.statusOptions"
              :key="status.value"
              @click="handleStatusChange(status)"
            >
              {{ statusDisplayLabel(status) }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <div v-else>
          <Button variant="ghost" size="sm" class="px-2 text-xs text-muted-foreground">
            <span>{{ conversationStore.conversations.total }}</span>
          </Button>
        </div>

        <!-- Sort dropdown-menu -->
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" class="px-2 text-xs text-muted-foreground hover:text-foreground gap-1">
              {{ conversationStore.getListSortField }}
              <ChevronDown class="size-3 opacity-50" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem @click="handleSortChange('oldest')">
              {{ $t('conversation.sort.oldestActivity') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('newest')">
              {{ $t('conversation.sort.newestActivity') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('started_first')">
              {{ $t('conversation.sort.startedFirst') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('started_last')">
              {{ $t('conversation.sort.startedLast') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('waiting_longest')">
              {{ $t('conversation.sort.waitingLongest') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('next_sla_target')">
              {{ $t('conversation.sort.nextSLATarget') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click="handleSortChange('priority_first')">
              {{ $t('conversation.sort.priorityFirst') }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <!-- Content -->
      <div class="min-h-0 flex-1 overflow-y-auto">
        <EmptyList
          v-if="!hasConversations && !hasErrored && !isLoading"
          key="empty"
          class="px-4 py-8"
          :title="t('conversation.noConversationsFound')"
          :message="t('conversation.tryAdjustingFilters')"
          :icon="MessageCircleQuestion"
        />

        <!-- Error State -->
        <EmptyList
          v-if="conversationStore.conversations.errorMessage"
          key="error"
          class="px-4 py-8"
          :title="t('conversation.couldNotFetch')"
          :message="conversationStore.conversations.errorMessage"
          :icon="MessageCircleWarning"
        />

        <!-- Empty State -->
        <TransitionGroup
          enter-active-class="transition-all duration-300 ease-in-out"
          enter-from-class="opacity-0 transform translate-y-4"
          enter-to-class="opacity-100 transform translate-y-0"
          leave-active-class="transition-all duration-300 ease-in-out"
          leave-from-class="opacity-100 transform translate-y-0"
          leave-to-class="opacity-0 transform translate-y-4"
        >
          <!-- Conversation List -->
          <div
            v-if="!conversationStore.conversations.errorMessage"
            key="list"
            class="divide-y divide-border"
            :class="{ 'border-b border-border': hasConversations }"
          >
            <ConversationListItem
              v-for="conversation in conversationStore.conversationsList"
              :key="conversation.uuid"
              :conversation="conversation"
              :currentConversation="conversationStore.current"
              :contactFullName="conversationStore.getContactFullName(conversation.uuid)"
              class="transition-colors duration-200"
            />
          </div>

          <!-- Loading Skeleton -->
          <div v-if="isLoading" key="loading" class="space-y-4">
            <ConversationListItemSkeleton v-for="index in 5" :key="index" />
          </div>
        </TransitionGroup>

        <!-- Load More -->
        <div
          v-if="!hasErrored && (conversationStore.conversations.hasMore || hasConversations)"
          class="flex justify-center items-center p-5"
        >
          <Button
            v-if="conversationStore.conversations.hasMore"
            variant="outline"
            @click="loadNextPage"
            :disabled="isLoading"
            class="transition-all duration-200 ease-in-out transform hover:scale-105"
          >
            <Loader v-if="isLoading" class="mr-2 h-4 w-4 animate-spin" />
            {{ isLoading ? t('globals.terms.loading') : t('globals.terms.loadMore') }}
          </Button>
          <p
            class="text-sm text-muted-foreground"
            v-else-if="conversationStore.conversationsList.length > 10"
          >
            {{ $t('conversation.allLoaded') }}
          </p>
        </div>
      </div>
    </template>

    <!-- floating view toggle, bottom-left of the list area -->
    <div class="absolute bottom-3 left-3 mb-2 z-10">
      <InboxViewToggle />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useConversationStore } from '../../../stores/conversation'
import { MessageQuestion as MessageCircleQuestion, MessageExclamation as MessageCircleWarning, ChevronDown, LayoutLeft, Loader } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import EmptyList from '@/features/conversation/list/ConversationEmptyList.vue'
import ConversationListItem from '@/features/conversation/list/ConversationListItem.vue'
import ConversationTable from '@/features/conversation/list/ConversationTable.vue'
import InboxViewToggle from '@/features/conversation/list/InboxViewToggle.vue'
import { useInboxView } from '@main/composables/useInboxView'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ConversationListItemSkeleton from '@/features/conversation/list/ConversationListItemSkeleton.vue'

import { getStatusDisplayLabel } from '@main/utils/statusVisuals'

const conversationStore = useConversationStore()
const route = useRoute()
const { t } = useI18n()
const { toggle: toggleSidebar } = useLayoutSidebar()
const { isTableView } = useInboxView()
const statusDisplayLabel = (status) => getStatusDisplayLabel(status, t)
const listStatusLabel = computed(() => statusDisplayLabel(conversationStore.getListStatus))

const title = computed(() => {
  const typeKey = route.meta?.typeKey?.(route)
  if (typeKey) {
    return t(typeKey)
  }
  const key = route.meta?.titleKey
  if (!key) return ''
  return t(key, route.meta?.titleCount || 1)
})

const handleStatusChange = (status) => {
  conversationStore.setListStatus(status.label)
}

const handleSortChange = (order) => {
  conversationStore.setListSortField(order)
}

const loadNextPage = () => {
  conversationStore.fetchNextConversations()
}

const hasConversations = computed(() => conversationStore.conversationsList.length !== 0)
const hasErrored = computed(() => !!conversationStore.conversations.errorMessage)
const isLoading = computed(() => conversationStore.conversations.loading)
</script>
