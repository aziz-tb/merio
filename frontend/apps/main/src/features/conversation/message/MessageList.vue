<!-- MessageList.vue -->
<template>
  <div class="flex flex-col relative h-full">
    <div ref="threadEl" class="flex-1 overflow-y-auto" @scroll="handleScroll">
      <div class="min-h-full px-4 pb-10">
        <div
          class="text-center mt-3"
          v-if="
            conversationStore.currentConversationHasMoreMessages &&
            !conversationStore.messages.loading
          "
        >
          <Button
            size="sm"
            variant="outline"
            @click="conversationStore.fetchNextMessages"
            class="transition-all duration-200 hover:bg-gray-100 dark:hover:bg-gray-700 hover:scale-105 active:scale-95"
          >
            <RefreshCw size="17" class="mr-2" />
            {{ $t('globals.terms.loadMore') }}
          </Button>
        </div>

        <MessagesSkeleton :count="10" v-if="conversationStore.messages.loading" />

        <TransitionGroup
          v-else
          enter-active-class="animate-slide-in"
          tag="div"
          class="divide-y divide-border/40"
        >
          <div v-for="group in groupedMessages" :key="group.id">
            <!-- Activity group: collapsed summary OR expanded list -->
            <div
              v-if="group.kind === 'activities'"
              class="py-2.5 px-4 text-center"
            >
              <button
                v-if="shouldCollapseActivities(group) && !isActivityGroupExpanded(group.id)"
                type="button"
                @click="toggleActivityGroup(group.id)"
                class="inline-flex items-center gap-1.5 text-xs text-muted-foreground bg-muted/50 hover:bg-muted px-2.5 py-1 rounded-full transition-colors"
              >
                <ClockRotateLeft :size="12" />
                {{ activityGroupSummary(group) }}
                <ChevronDown :size="12" />
              </button>
              <div v-else>
                <ActivityMessageBubble
                  v-for="activityMessage in group.items"
                  :key="activityMessage.uuid"
                  :message="activityMessage"
                />
                <button
                  v-if="shouldCollapseActivities(group)"
                  type="button"
                  @click="toggleActivityGroup(group.id)"
                  class="my-2 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  <ChevronUp :size="11" />
                  {{ $t('conversation.collapse') }}
                </button>
              </div>
            </div>

            <!-- Regular message -->
            <div
              v-else
              :data-message-uuid="group.item.uuid"
            >
              <div v-if="!group.item.private && group.item.type !== 'activity'">
                <MessageBubble :message="group.item" :direction="group.item.type" />
              </div>
              <div v-else-if="isPrivateNote(group.item)">
                <MessageBubble :message="group.item" direction="outgoing" />
              </div>
            </div>
          </div>
        </TransitionGroup>
      </div>

      <!-- Typing indicator -->
      <div v-if="conversationStore.conversation.isTyping" class="px-4 pb-4">
        <TypingIndicator />
      </div>
    </div>

    <!-- Sticky container for the scroll arrow -->
    <ScrollToBottomButton
      :is-at-bottom="isAtBottom"
      :unread-count="unReadMessages"
      @scroll-to-bottom="handleScrollToBottom"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import MessageBubble from './MessageBubble.vue'
import ActivityMessageBubble from './ActivityMessageBubble.vue'
import { useConversationStore } from '@main/stores/conversation'
import { useUserStore } from '@main/stores/user'
import { Button } from '@shared-ui/components/ui/button'
import {
  ArrowsRotate as RefreshCw,
  ChevronDown,
  ChevronUp,
  ClockRotateLeft
} from '@qall/icons'
import ScrollToBottomButton from '@shared-ui/components/ScrollToBottomButton'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents'
import MessagesSkeleton from './MessagesSkeleton.vue'
import { TypingIndicator } from '@shared-ui/components/TypingIndicator'
import { formatMessageTimestamp } from '@shared-ui/utils/datetime.js'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const { t } = useI18n()

const conversationStore = useConversationStore()
const userStore = useUserStore()
const threadEl = ref(null)
const emitter = useEmitter()
const isAtBottom = ref(true)
const unReadMessages = ref(0)
const currentConversationUUID = ref('')

// Activities collapse below this threshold are shown expanded inline; clusters
// of 3+ events collapse into a single chip with count and date range.
const ACTIVITY_COLLAPSE_THRESHOLD = 2
const expandedActivityGroups = ref(new Set())

// Group consecutive activity messages into one "activities" block. Regular
// messages stay as individual groups. The result is what the template
// iterates so hairlines (via divide-y on the parent) sit between groups,
// not between individual activity rows.
const groupedMessages = computed(() => {
  const messages = conversationStore.conversationMessages
  const groups = []
  let currentActivityGroup = null

  for (const message of messages) {
    if (message.type === 'activity') {
      if (!currentActivityGroup) {
        // Stable id from the first activity in the run so expansion state and
        // transition keys survive older-message prepends ("Load more").
        currentActivityGroup = { kind: 'activities', id: 'act-' + message.uuid, items: [] }
        groups.push(currentActivityGroup)
      }
      currentActivityGroup.items.push(message)
    } else {
      currentActivityGroup = null
      groups.push({ kind: 'message', id: message.uuid, item: message })
    }
  }

  return groups
})

function isActivityGroupExpanded(groupId) {
  return expandedActivityGroups.value.has(groupId)
}

function toggleActivityGroup(groupId) {
  const set = expandedActivityGroups.value
  if (set.has(groupId)) set.delete(groupId)
  else set.add(groupId)
  // Re-assign to trigger reactivity since Set mutations aren't observed
  expandedActivityGroups.value = new Set(set)
}

function shouldCollapseActivities(group) {
  return group.items.length >= ACTIVITY_COLLAPSE_THRESHOLD
}

function activityGroupSummary(group) {
  const first = group.items[0]
  const last = group.items[group.items.length - 1]
  const firstDate = formatMessageTimestamp(first.updated_at).split(',')[0]
  const lastDate = formatMessageTimestamp(last.updated_at).split(',')[0]
  if (firstDate === lastDate) {
    return t('conversation.systemEventsOn', {
      count: group.items.length,
      date: firstDate
    })
  }
  return t('conversation.systemEventsBetween', {
    count: group.items.length,
    firstDate,
    lastDate
  })
}

const checkIfAtBottom = () => {
  const thread = threadEl.value
  if (thread) {
    const tolerance = 100
    const isBottom = thread.scrollHeight - thread.scrollTop - thread.clientHeight <= tolerance
    isAtBottom.value = isBottom
  }
}

const handleScroll = () => {
  checkIfAtBottom()
}

const handleScrollToBottom = () => {
  scrollToBottom()
}

const scrollToBottom = () => {
  setTimeout(() => {
    const thread = threadEl.value
    if (thread) {
      thread.scrollTop = thread.scrollHeight
      checkIfAtBottom()
    }
  }, 50)
}

const scrollToMessage = (messageUUID) => {
  if (!messageUUID) {
    scrollToBottom()
    return
  }

  setTimeout(() => {
    const thread = threadEl.value
    const messageEl = thread?.querySelector(`[data-message-uuid="${messageUUID}"]`)
    if (messageEl && thread) {
      // Manual scroll calculation for reliability with variable-height messages
      const messageTop = messageEl.offsetTop
      const threadHeight = thread.clientHeight
      const messageHeight = messageEl.offsetHeight
      // Position message at ~1/3 from top of viewport for better visibility
      const targetScroll = messageTop - threadHeight / 3 + messageHeight / 2
      thread.scrollTop = Math.max(0, targetScroll)

      // Highlight the message briefly
      messageEl.classList.add('highlight-mention')
      setTimeout(() => messageEl.classList.remove('highlight-mention'), 2500)
    } else {
      // Message not found, scroll to bottom instead
      scrollToBottom()
    }
  }, 150)
}

onMounted(() => {
  checkIfAtBottom()
  handleNewMessage()
})

const newMessageHandler = (data) => {
  if (data.conversation_uuid === conversationStore.current.uuid) {
    // Agent's own message - always scroll to bottom
    if (data.message?.sender_id === userStore.userID) {
      scrollToBottom()
    }
    // Customer message - only scroll if already at bottom
    else if (isAtBottom.value) {
      scrollToBottom()
    }
    // Customer message but not at bottom - don't scroll, increment unread
    else {
      unReadMessages.value++
    }
  }
}

const handleNewMessage = () => {
  emitter.on(EMITTER_EVENTS.NEW_MESSAGE, newMessageHandler)
}

onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.NEW_MESSAGE, newMessageHandler)
})

watch(
  () => conversationStore.conversationMessages,
  (messages) => {
    // Scroll to bottom when conversation changes and there are new messages.
    // New messages on next db page should not scroll to bottom.
    if (
      messages.length > 0 &&
      conversationStore?.current?.uuid &&
      currentConversationUUID.value !== conversationStore.current.uuid
    ) {
      currentConversationUUID.value = conversationStore.current.uuid
      unReadMessages.value = 0
      // Reset activity group expansion state for fresh conversation
      expandedActivityGroups.value = new Set()

      // Check if this is a mentioned conversation
      const scrollToUUID = route.query.scrollTo
      if (scrollToUUID) {
        // Mentioned conversation - only scroll to message, NOT to bottom
        scrollToMessage(scrollToUUID)
      } else {
        // Normal conversation - scroll to bottom
        scrollToBottom()
      }
    }
  }
)

// Watch for typing indicator and auto-scroll if user is at bottom
watch(
  () => conversationStore.conversation.isTyping,
  (isTyping) => {
    if (isTyping && isAtBottom.value) {
      scrollToBottom()
    }
  }
)

// Watch for isAtButtom and set unReadMessages to 0
watch(
  () => isAtBottom.value,
  (atBottom) => {
    if (atBottom) {
      unReadMessages.value = 0
    }
  }
)

const isPrivateNote = (message) => {
  return message.type === 'outgoing' && message.private
}
</script>

<style scoped>
.highlight-mention {
  animation: highlightPulse 2.5s ease-out;
}

@keyframes highlightPulse {
  0% {
    background-color: rgb(251 191 36 / 0.35);
    border-radius: 0.5rem;
  }
  100% {
    background-color: transparent;
  }
}

/* Dark mode highlight - softer yellow */
:global(.dark) .highlight-mention {
  animation: highlightPulseDark 2.5s ease-out;
}

@keyframes highlightPulseDark {
  0% {
    background-color: rgb(250 204 21 / 0.2);
    border-radius: 0.5rem;
  }
  100% {
    background-color: transparent;
  }
}
</style>