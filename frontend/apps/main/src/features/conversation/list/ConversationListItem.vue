<template>
  <ContextMenu>
    <ContextMenuTrigger asChild>
      <router-link
        :to="conversationRoute"
        class="group relative block px-4 py-3 transition-colors duration-150 cursor-pointer hover:bg-sidebar-accent/60"
        :class="{
          'bg-sidebar-accent': conversation.uuid === currentConversation?.uuid
        }"
      >
        <div class="flex items-start gap-3">
          <!-- Avatar with channel indicator -->
          <div class="relative flex-shrink-0">
            <Avatar class="w-7 h-7 rounded-full">
              <AvatarImage
                :src="conversation.contact.avatar_url || ''"
                class="object-cover"
              />
              <AvatarFallback class="text-[10px]">
                {{ conversation.contact.first_name.substring(0, 2).toUpperCase() }}
              </AvatarFallback>
            </Avatar>
            <span class="absolute -bottom-0.5 -right-0.5 flex items-center justify-center w-3.5 h-3.5 rounded-full bg-background border border-border">
              <component :is="conversation.inbox_channel === 'livechat' ? MessageSquare : Mail" class="w-2 h-2 text-muted-foreground" />
            </span>
          </div>

          <!-- Content container -->
          <div class="flex-1 min-w-0 space-y-1">
            <!-- Contact name + time -->
            <div class="flex items-baseline justify-between gap-2">
              <h3 class="text-sm font-semibold truncate text-foreground">
                {{ contactFullName }}
              </h3>
              <span
                class="text-xs text-muted-foreground whitespace-nowrap flex-shrink-0 tabular-nums"
                v-if="conversation.last_message_at"
              >
                {{ relativeLastMessageTime }}
              </span>
            </div>

            <!-- Message preview + unread count -->
            <div class="flex items-center justify-between gap-2">
              <p class="text-xs flex-1 min-w-0 truncate text-muted-foreground">
                <template v-if="hasDraftForConversation">
                  <FilePen
                    class="text-primary inline-block align-text-bottom mr-1"
                    :size="12"
                  /><span class="italic mr-1">{{ $t('globals.terms.draft') }}</span>{{ draftPreview }}
                </template>
                <template v-else>
                  <Reply
                    class="text-muted-foreground inline-block align-text-bottom mr-0.5"
                    :size="12"
                    v-if="conversation.last_message_sender === 'agent'"
                  />{{ trimmedLastMessage }}
                </template>
              </p>
              <div
                v-if="conversation.unread_message_count > 0"
                class="flex items-center justify-center min-w-5 h-5 px-1 bg-primary text-primary-foreground text-[10px] font-medium rounded-full flex-shrink-0"
              >
                {{ conversation.unread_message_count > 9 ? '9+' : conversation.unread_message_count }}
              </div>
            </div>

            <!-- SLA Badges -->
            <div v-if="hasSlaDeadlines" class="flex items-center gap-1">
              <SlaBadge
                v-show="frdStatus === 'overdue' || frdStatus === 'remaining'"
                :dueAt="conversation.first_response_deadline_at"
                :actualAt="conversation.first_reply_at"
                :label="'FRD'"
                :showExtra="false"
                @status="frdStatus = $event"
                :key="`${conversation.uuid}-${conversation.first_response_deadline_at}-${conversation.first_reply_at}`"
              />
              <SlaBadge
                v-show="rdStatus === 'overdue' || rdStatus === 'remaining'"
                :dueAt="conversation.resolution_deadline_at"
                :actualAt="conversation.resolved_at"
                :label="'RD'"
                :showExtra="false"
                @status="rdStatus = $event"
                :key="`${conversation.uuid}-${conversation.resolution_deadline_at}-${conversation.resolved_at}`"
              />
              <SlaBadge
                v-show="nrdStatus === 'overdue' || nrdStatus === 'remaining'"
                :dueAt="conversation.next_response_deadline_at"
                :actualAt="conversation.next_response_met_at"
                :label="'NRD'"
                :showExtra="false"
                @status="nrdStatus = $event"
                :key="`${conversation.uuid}-${conversation.next_response_deadline_at}-${conversation.next_response_met_at}`"
              />
            </div>
          </div>
        </div>
      </router-link>
    </ContextMenuTrigger>
    <ContextMenuContent>
      <ContextMenuItem @click="handleMarkAsUnread">
        <MailOpen class="w-4 h-4 mr-2" />
        {{ $t('globals.messages.markAsUnread') }}
      </ContextMenuItem>
    </ContextMenuContent>
  </ContextMenu>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { getRelativeTime } from '@shared-ui/utils/datetime.js'
import { Mail, MessageSquare, Reply, MailOpen, FilePen } from 'lucide-vue-next'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger
} from '@shared-ui/components/ui/context-menu'
import SlaBadge from '@main/features/sla/SlaBadge.vue'
import { useConversationStore } from '@main/stores/conversation'

let timer = null
const now = ref(new Date())
const route = useRoute()
const conversationStore = useConversationStore()
const frdStatus = ref('')
const rdStatus = ref('')
const nrdStatus = ref('')

const props = defineProps({
  conversation: Object,
  currentConversation: Object,
  contactFullName: String
})

const handleMarkAsUnread = () => {
  conversationStore.markAsUnread(props.conversation.uuid)
}

const conversationRoute = computed(() => {
  const baseRoute = route.name.includes('team')
    ? 'team-inbox-conversation'
    : route.name.includes('view')
      ? 'view-inbox-conversation'
      : 'inbox-conversation'
  return {
    name: baseRoute,
    params: {
      uuid: props.conversation.uuid,
      ...(baseRoute === 'team-inbox-conversation' && { teamID: route.params.teamID }),
      ...(baseRoute === 'view-inbox-conversation' && { viewID: route.params.viewID })
    },
    query: props.conversation.mentioned_message_uuid
      ? { scrollTo: props.conversation.mentioned_message_uuid }
      : {}
  }
})

onMounted(() => {
  timer = setInterval(() => {
    now.value = new Date()
  }, 60000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const trimmedLastMessage = computed(() => {
  const message = props.conversation.last_message || ''
  return message.length > 120 ? message.slice(0, 120) + '...' : message
})

const relativeLastMessageTime = computed(() => {
  return props.conversation.last_message_at
    ? getRelativeTime(props.conversation.last_message_at, now.value)
    : ''
})

const hasSlaDeadlines = computed(() => {
  const c = props.conversation
  return c.first_response_deadline_at || c.resolution_deadline_at || c.next_response_deadline_at
})

const hasDraftForConversation = computed(() => {
  return conversationStore.hasDraft(props.conversation.uuid)
})

const draftPreview = computed(() => {
  const draft = conversationStore.getDraft(props.conversation.uuid)
  if (!draft?.content) return ''
  const text = draft.content.replace(/<[^>]*>/g, '').trim()
  return text.length > 120 ? text.slice(0, 120) + '...' : text
})
</script>
