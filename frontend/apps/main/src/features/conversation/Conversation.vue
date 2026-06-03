<template>
  <div class="flex flex-col h-full bg-background">
    <!-- Header -->
    <div class="h-14 flex-shrink-0 px-4 border-b border-border flex items-center justify-between gap-3">
      <!-- Left: contact name + optional subject -->
      <div class="flex flex-col min-w-0">
        <span v-if="conversationStore.current?.subject" class="text-base font-semibold truncate">
          
          {{ conversationStore.current.subject }}
        </span>
        <Skeleton class="w-[130px] h-5" v-else />
        <span
          v-if="!conversationStore.conversation.loading" 
          class="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground"
        >
          <span class="truncate">{{ currentDisplayName }}</span>
          <Tooltip v-if="currentHasLinkedCustomer">
            <TooltipTrigger as-child>
              <LinkSimple class="size-3 shrink-0 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent>{{ t('conversation.customer.linkedTooltip') }}</TooltipContent>
          </Tooltip>
        </span>
      </div>

      <!-- Right cluster -->
      <div class="flex items-center gap-1 flex-shrink-0">
        <Skeleton v-if="conversationStore.conversation.loading" class="w-[80px] h-8 rounded-full" />

        <template v-else>
          <!-- Other agents currently viewing this conversation. Up to 3 small avatars + "+N" badge. -->
          <div v-if="otherViewers.length > 0" class="flex items-center mr-1">
            <Tooltip v-for="(viewer, idx) in otherViewers.slice(0, 3)" :key="viewer.id">
              <TooltipTrigger as-child>
                <Avatar :class="['w-6 h-6 ring-2 ring-background', idx > 0 ? '-ml-2' : '']">
                  <AvatarImage :src="viewer.avatar_url || ''" />
                  <AvatarFallback class="text-[10px] font-medium">
                    {{ (viewer.first_name || '?').toUpperCase().substring(0, 2) }}
                  </AvatarFallback>
                </Avatar>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                <p>{{ [viewer.first_name, viewer.last_name].filter(Boolean).join(' ') || 'Agent' }}</p>
              </TooltipContent>
            </Tooltip>
            <span
              v-if="otherViewers.length > 3"
              class="-ml-2 inline-flex items-center justify-center w-6 h-6 rounded-full ring-2 ring-background bg-muted text-[10px] font-medium text-muted-foreground"
            >
              +{{ otherViewers.length - 3 }}
            </span>
          </div>

          <!-- Status pill — icon + label always inline. Dropdown anchors to this Button. -->
          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <StatusPill
                as="button"
                type="button"
                :status="currentStatusOption"
                :label="statusDisplayLabel(currentStatusOption)"
                size="md"
                class="shrink-0 cursor-pointer transition-opacity hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-1"
              />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" class="min-w-44">
              <DropdownMenuItem
                v-for="status in conversationStore.statusOptions"
                :key="status.value"
                class="gap-2"
                @click="handleUpdateStatus(status)"
              >
                <StatusPill :status="status" :label="statusDisplayLabel(status)" size="sm" />
                <Check
                  :class="[
                    'ml-auto size-4',
                    isSelectedStatus(status) ? 'opacity-100' : 'opacity-0'
                  ]"
                />
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <!-- Details panel toggle — replaces the old bottom toggle on the conversation list. -->
          <button
            type="button"
            class="inline-flex items-center justify-center size-8 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            :title="detailsPaneOpen ? t('conversation.layoutTwoPane') : t('conversation.layoutThreePane')"
            @click="detailsPaneOpen = !detailsPaneOpen"
          >
            <LayoutRight class="size-4" />
          </button>
        </template>
      </div>
    </div>

    <!-- Messages & reply box -->
    <div class="flex flex-col flex-grow overflow-hidden">
      <MessageList class="flex-1 overflow-y-auto" />
      <ReplyBox />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useStorage } from '@vueuse/core'
import { useConversationStore } from '../../stores/conversation'
import { useUserStore } from '@main/stores/user'
import { useUsersStore } from '@main/stores/users'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import StatusPill from '@main/components/status/StatusPill.vue'
import MessageList from '@/features/conversation/message/MessageList.vue'
import ReplyBox from './ReplyBox.vue'
import { EMITTER_EVENTS } from '../../constants/emitterEvents.js'
import { CONVERSATION_DEFAULT_STATUSES } from '../../constants/conversation'
import { useEmitter } from '../../composables/useEmitter'
import { Skeleton } from '@shared-ui/components/ui/skeleton'
import { getStatusDisplayLabel } from '@main/utils/statusVisuals'
import { getConversationDisplayName, hasLinkedCustomer } from '@main/utils/conversationDisplay'
import { Check, LayoutRight, Link, LinkSimple } from '@qall/icons'

const conversationStore = useConversationStore()
const userStore = useUserStore()
const usersStore = useUsersStore()
const emitter = useEmitter()
const { t } = useI18n()
const detailsPaneOpen = useStorage('conversationSidebarOpen', true)

const statusButtonLabel = computed(() => conversationStore.current?.status || '')
const currentDisplayName = computed(() => getConversationDisplayName(conversationStore.current))
const currentHasLinkedCustomer = computed(() => hasLinkedCustomer(conversationStore.current))
const currentStatusOption = computed(() =>
  conversationStore.statusOptions.find((status) => status.label === statusButtonLabel.value) || {
    label: statusButtonLabel.value
  }
)
const statusDisplayLabel = (status) => getStatusDisplayLabel(status, t)
const isSelectedStatus = (status) => status?.label === statusButtonLabel.value

// Other agents currently viewing this conversation (excludes self).
// Reactively reads the conversation store's `viewers` map (updated by the WS handler) and
// resolves user objects via the already-loaded users store.
const otherViewers = computed(() => {
  const cuuid = conversationStore.current?.uuid
  if (!cuuid) return []
  const ids = conversationStore.viewers?.[cuuid] || []
  const me = userStore.userID
  return ids
    .filter((id) => id !== me)
    .map((id) => usersStore.users?.find?.((u) => u.id === id))
    .filter(Boolean)
})

const handleUpdateStatus = (status) => {
  const statusLabel = status?.label || status
  if (statusLabel === CONVERSATION_DEFAULT_STATUSES.SNOOZED) {
    emitter.emit(EMITTER_EVENTS.SET_NESTED_COMMAND, {
      command: 'snooze',
      open: true
    })
    return
  }
  conversationStore.updateStatus(statusLabel)
}
</script>
