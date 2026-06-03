<template>
  <!-- Inline message row (no bubble bg). Borders come from `divide-y` on the parent list, so single messages have no border and stacks have hairlines only between adjacent rows. -->
  <div
    class="group flex flex-row gap-3 pl-2 px-4 py-4 w-full"
    :class="{ 'bg-private': isPrivateMessage }"
  >
    <!-- Avatar -->
    <Avatar class="w-8 h-8 mt-1 flex-shrink-0">
      <AvatarImage :src="getAvatar" />
      <AvatarFallback class="text-xs font-medium">
        <User v-if="!isOutgoing" class="size-4 text-muted-foreground" />
        <span v-else>{{ avatarFallback }}</span>
      </AvatarFallback>
    </Avatar>

    <!-- Content -->
    <div class="flex-1 min-w-0">
      <!-- Header: name (+ meta line) on the left, timestamp on the right -->
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <p class="font-semibold text-sm truncate">{{ getFullName }}</p>
          <MessageEnvelope
            v-if="showEnvelope"
            :message="message"
            :direction="direction"
            class="mt-0.5"
          />
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <p class="text-xs text-muted-foreground tabular-nums cursor-default">{{ formatMessageTimestamp(message.created_at) }}</p>
        </div>
      </div>

      <!-- Body -->
      <div class="mt-3 text-sm">
        <div
          v-if="message.content_type === 'text'"
          class="native-html whitespace-pre-wrap"
          :class="{ 'mb-3': message.attachments.length > 0 }"
        >
          {{ sanitizedContent }}
        </div>
        <Letter
          v-else
          :html="sanitizedContent"
          :allowedSchemas="['cid', 'https', 'http', 'mailto']"
          class="native-html break-words"
          :class="{
            'mb-3': message.attachments.length > 0,
            'show-quoted-text': showQuotedText,
            'hide-quoted-text': !showQuotedText,
            'show-forwarded-text': showForwardedText,
            'hide-forwarded-text': !showForwardedText
          }"
        />

        <!-- Forwarded Text Toggle: Gmail-style [...] indicator that expands/collapses
             every <div data-source-uuid="..."> wrapper inserted by the forward composer.
             Default collapsed so the agent's note stays prominent and the bulk thread is one tap away. -->
        <button
          v-if="hasForwardedContent"
          type="button"
          @click="toggleForwarded"
          :title="showForwardedText ? t('conversation.hideForwardedMessages') : t('conversation.showForwardedMessages')"
          class="inline-flex items-center justify-center mt-2 px-2 py-0.5 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-200"
        >
          <MoreHorizontal class="size-4" />
        </button>

        <!-- Quoted Text Toggle (incoming only) -->
        <div
          v-if="!isOutgoing && hasQuotedContent"
          @click="toggleQuote"
          class="text-xs cursor-pointer text-muted-foreground px-2 py-1 w-max mt-2 hover:bg-muted hover:text-primary rounded transition-colors duration-200"
        >
          {{ showQuotedText ? t('conversation.hideQuotedText') : t('conversation.showQuotedText') }}
        </div>

        <!-- Attachments -->
        <MessageAttachmentPreview :attachments="nonInlineAttachments" />

        <!-- CSAT Response -->
        <CSATResponseDisplay :message="message" />

        <!-- Spinner for Pending Messages (outgoing only) -->
        <Spinner v-if="isOutgoing && message.status === 'pending'" size="sm" />
      </div>

      <!-- Status Icons (outgoing only) -->
      <div v-if="isOutgoing" class="flex items-center gap-2 mt-2">
        <Lock :size="10" v-if="isPrivateMessage" class="text-muted-foreground" />
        <Check :size="14" v-if="showCheckCheck" class="text-green-500" />
        <Tooltip v-if="message.meta?.continuity_emailed">
          <TooltipTrigger>
            <Mail :size="12" class="text-muted-foreground" />
          </TooltipTrigger>
          <TooltipContent>
            <p>{{ t('conversation.sentViaEmail') }}</p>
          </TooltipContent>
        </Tooltip>
        <RotateCcw
          size="10"
          @click="retryMessage(message)"
          class="cursor-pointer text-muted-foreground hover:text-foreground transition-colors duration-200"
          v-if="showRetry"
        />
      </div>
    </div>
  </div>

</template>

<script setup>
import { computed, ref } from 'vue'
import { useConversationStore } from '@main/stores/conversation'
import { useUserStore } from '@main/stores/user'
import { useI18n } from 'vue-i18n'
import { Lock, Envelope as Mail, ArrowRotateLeft as RotateCcw, Check, Ellipsis as MoreHorizontal, User } from '@qall/icons'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { formatMessageTimestamp } from '@shared-ui/utils/datetime.js'
import { containsQuoteMarkers } from '@shared-ui/utils/quotedContent'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import { Letter } from 'vue-letter'
import MessageAttachmentPreview from '@main/features/conversation/message/attachment/MessageAttachmentPreview.vue'
import MessageEnvelope from './MessageEnvelope.vue'
import CSATResponseDisplay from './CSATResponseDisplay.vue'
import api from '@main/api'
import { getConversationDisplayName } from '@main/utils/conversationDisplay'

const props = defineProps({
  message: Object,
  direction: {
    type: String,
    validator: (v) => ['incoming', 'outgoing'].includes(v)
  }
})

const convStore = useConversationStore()
const { t } = useI18n()
const userStore = useUserStore()

const isOutgoing = computed(() => props.direction === 'outgoing')

const getFullName = computed(() => {
  if (!isOutgoing.value) {
    return getConversationDisplayName(convStore.current)
  }
  const author = props.message.author ?? {}
  const firstName = author.first_name ?? 'User'
  const lastName = author.last_name ?? ''
  return `${firstName} ${lastName}`.trim()
})

const getAvatar = computed(() => props.message.author?.avatar_url || '')

const avatarFallback = computed(() => {
  const firstName = props.message.author?.first_name ?? (isOutgoing.value ? 'A' : 'U')
  return firstName.toUpperCase().substring(0, 2)
})

const sanitizedContent = computed(() => {
  if (props.message.meta?.is_csat) {
    return t('globals.messages.pleaseRateConversation')
  }
  return props.message.content || ''
})

const nonInlineAttachments = computed(() =>
  props.message.attachments.filter((attachment) => attachment.disposition !== 'inline')
)

const isPrivateMessage = computed(() => props.message.private)
const showCheckCheck = computed(
  () => isOutgoing.value && props.message.status === 'sent' && !isPrivateMessage.value
)
const showRetry = computed(() => isOutgoing.value && props.message.status === 'failed' && props.message.sender_id === userStore.userID)

const retryMessage = (msg) => {
  api.retryMessage(convStore.current.uuid, msg.uuid)
}

// Incoming-only: quoted text toggle
const showQuotedText = ref(false)
const hasQuotedContent = computed(
  () => !isOutgoing.value && containsQuoteMarkers(sanitizedContent.value)
)
const toggleQuote = () => {
  showQuotedText.value = !showQuotedText.value
}

// Forwarded-block toggle: collapses/expands every <div data-source-uuid="..."> in the
// rendered message. Default collapsed so the agent's note is the focus.
const showForwardedText = ref(false)
const hasForwardedContent = computed(
  () => sanitizedContent.value.includes('data-source-uuid')
)
const toggleForwarded = () => {
  showForwardedText.value = !showForwardedText.value
}

// Envelope visibility (both directions)
const showEnvelope = computed(() => {
  return (
    props.message.meta?.from?.length ||
    props.message.meta?.to?.length ||
    props.message.meta?.cc?.length ||
    props.message.meta?.bcc?.length ||
    props.message.meta?.subject
  )
})

</script>
