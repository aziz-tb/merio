<template>
  <!-- Set fixed width only when not in fullscreen. -->
  <div class="flex flex-col h-full" :class="{ 'max-h-[600px]': !isFullscreen }">
    <!-- Top row: compact mode dropdown (reply / forward / private note) on the left,
         fullscreen toggle on the right. The dropdown label tracks the active messageType. -->
    <div
      class="flex justify-between items-center"
      :class="{ 'mb-1': !isFullscreen, 'border-b border-border pb-2': isFullscreen }"
    >
      <div class="flex items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="inline-flex items-center gap-1.5 px-1 -ml-1 rounded text-sm font-semibold hover:bg-accent transition-colors"
              :class="messageType === 'private_note' ? 'text-amber-700' : 'text-foreground'"
            >
              <component :is="modeIcon" class="size-4" />
              <span>{{ modeLabel }}</span>
              <ChevronDown class="size-3.5 opacity-60" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" class="w-44">
            <DropdownMenuItem @click="setMode('reply')">
              <MessageSquare class="size-4 mr-2" />
              <span>{{ $t('globals.terms.reply') }}</span>
            </DropdownMenuItem>
            <DropdownMenuItem v-if="canForward" @click="setMode('forward')">
              <MessageSquareShare class="size-4 mr-2" />
              <span>{{ $t('globals.terms.forward') }}</span>
            </DropdownMenuItem>
            <DropdownMenuItem @click="setMode('private_note')">
              <MessageSquareText class="size-4 mr-2" />
              <span>{{ $t('globals.terms.privateNote') }}</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <!-- Forward status chip: live count of <div data-source-uuid> blocks still in the editor. -->
        <span
          v-if="messageType === 'forward' && forwardSourceCount > 0"
          class="inline-flex items-center text-xs text-muted-foreground bg-muted/50 rounded px-2 py-0.5"
        >
          {{ $t('replyBox.forwardingMessages', forwardSourceCount, { count: forwardSourceCount }) }}
        </span>
      </div>
      <Button class="text-muted-foreground" variant="ghost" size="icon" @click="toggleFullscreen">
        <component :is="isFullscreen ? Minimize2 : Maximize2" class="size-4" />
      </Button>
    </div>

    <!-- To, CC, and BCC fields. Visible in reply + forward modes (email channel only). -->
    <div v-if="conversationStore.current.inbox_channel === 'email'">
      <div
        :class="['space-y-3', isFullscreen ? 'p-4 border-b border-border' : 'mb-4']"
        v-if="messageType === 'reply' || messageType === 'forward'"
      >
        <div class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">TO:</label>
          <Input
            type="text"
            :placeholder="t('replyBox.emailAddresess')"
            v-model="to"
            class="flex-grow px-3 py-2 text-sm border rounded focus:ring-2 focus:ring-ring"
            @blur="validateEmails"
          />
          <Button
            size="sm"
            @click="toggleCc"
            class="text-sm bg-secondary text-secondary-foreground hover:bg-secondary/80"
          >
            {{ showCc ? $t('replyBox.removeCC') : $t('replyBox.cc') }}
          </Button>
          <Button
            size="sm"
            @click="toggleBcc"
            class="text-sm bg-secondary text-secondary-foreground hover:bg-secondary/80"
          >
            {{ showBcc ? $t('replyBox.removeBCC') : $t('replyBox.bcc') }}
          </Button>
        </div>
        <div v-if="showCc" class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">CC:</label>
          <Input
            type="text"
            :placeholder="t('replyBox.emailAddresess')"
            v-model="cc"
            class="flex-grow px-3 py-2 text-sm border rounded focus:ring-2 focus:ring-ring"
            @blur="validateEmails"
          />
        </div>
        <div v-if="showBcc" class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">BCC:</label>
          <Input
            type="text"
            :placeholder="t('replyBox.emailAddresess')"
            v-model="bcc"
            class="flex-grow px-3 py-2 text-sm border rounded focus:ring-2 focus:ring-ring"
            @blur="validateEmails"
          />
        </div>
      </div>

      <!-- email errors -->
      <div
        v-if="emailErrors.length > 0"
        class="mb-4 px-2 py-1 bg-destructive/10 border border-destructive text-destructive rounded"
      >
        <p v-for="error in emailErrors" :key="error" class="text-sm">{{ error }}</p>
      </div>
    </div>

    <!-- Main tiptap editor -->
    <div class="flex-grow flex flex-col overflow-hidden">
      <Editor
        ref="editorRef"
        v-model:htmlContent="htmlContent"
        v-model:textContent="textContent"
        :message-type="messageType"
        :placeholder="editorPlaceholder"
        :aiPrompts="aiPrompts"
        :insertContent="insertContent"
        :autoFocus="true"
        :disabled="isDraftLoading"
        :enableMentions="messageType === 'private_note'"
        :getSuggestions="getSuggestions"
        @aiPromptSelected="handleAiPromptSelected"
        @send="handleSend"
        @mentionsChanged="handleMentionsChanged"
      />
    </div>

    <!-- Macro preview -->
    <MacroActionsPreview
      v-if="conversationStore.getMacro(MACRO_CONTEXT.REPLY)?.actions?.length > 0"
      :actions="conversationStore.getMacro(MACRO_CONTEXT.REPLY).actions"
      :onRemove="(action) => conversationStore.removeMacroAction(action, MACRO_CONTEXT.REPLY)"
      class="mt-2"
    />

    <!-- Attachments preview -->
    <AttachmentsPreview
      :attachments="uploadedFiles"
      :uploadingFiles="uploadingFiles"
      :onDelete="handleOnFileDelete"
      v-if="uploadedFiles.length > 0 || uploadingFiles.length > 0"
      class="mt-2"
    />

    <!-- Forward sources preview: non-editable cards rendered via v-html, kept out of Tiptap so
         email HTML survives. Click X to drop a source from the forward. -->
    <div
      v-if="messageType === 'forward' && forwardedSources.length > 0"
      class="mt-2 space-y-2 max-h-64 overflow-y-auto"
    >
      <div
        v-for="source in forwardedSources"
        :key="source.uuid"
        class="relative"
      >
        <button
          type="button"
          @click="removeSource(source.uuid)"
          :title="$t('globals.messages.remove')"
          class="absolute top-2 right-2 inline-flex items-center justify-center size-6 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors z-10"
        >
          <X class="size-3.5" />
        </button>
        <div class="native-html text-sm" v-html="sourcePreviewHTML(source)" />
      </div>
    </div>

    <!-- Editor menu bar with send button -->
    <ReplyBoxMenuBar
      class="mt-1 shrink-0"
      :isFullscreen="isFullscreen"
      :messageType="messageType"
      :handleFileUpload="handleFileUpload"
      :isSending="isSending"
      :enableSend="enableSend"
      :handleSend="handleSend"
      @emojiSelect="handleEmojiSelect"
    />
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch } from 'vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { MACRO_CONTEXT } from '@main/constants/conversation'
import {
  Maximize2,
  Minimize2,
  MessageSquareText,
  MessageSquare,
  MessageSquareShare,
  ChevronDown,
  X
} from 'lucide-vue-next'
import Editor from '@main/components/editor/TextEditor.vue'
import { useConversationStore } from '@main/stores/conversation'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { useEmitter } from '@main/composables/useEmitter'
import AttachmentsPreview from '@/features/conversation/message/attachment/AttachmentsPreview.vue'
import MacroActionsPreview from '@/features/conversation/MacroActionsPreview.vue'
import ReplyBoxMenuBar from '@/features/conversation/ReplyBoxMenuBar.vue'
import { useI18n } from 'vue-i18n'
import { buildForwardSourceHTML } from '@/features/conversation/forwardBody.js'
import { validateEmail } from '@shared-ui/utils/string'
import { useMacroStore } from '@main/stores/macro'
import { useUsersStore } from '@main/stores/users'
import { useTeamStore } from '@main/stores/team'

const messageType = defineModel('messageType', { default: 'reply' })
const to = defineModel('to', { default: '' })
const cc = defineModel('cc', { default: '' })
const bcc = defineModel('bcc', { default: '' })
const showCc = defineModel('showCc', { default: false })
const showBcc = defineModel('showBcc', { default: false })
const emailErrors = defineModel('emailErrors', { default: () => [] })
const htmlContent = defineModel('htmlContent', { default: '' })
const textContent = defineModel('textContent', { default: '' })
const mentions = defineModel('mentions', { default: () => [] })
// Sources to forward — populated when entering Forward mode, cleared on send/exit.
// Kept out of the Tiptap editor so email HTML doesn't get stripped by ProseMirror's parser.
const forwardedSources = defineModel('forwardedSources', { default: () => [] })
const macroStore = useMacroStore()
const usersStore = useUsersStore()
const teamStore = useTeamStore()

// Get suggestions for the mention dropdown
const getSuggestions = async (query) => {
  // Only show suggestions in private note mode
  if (messageType.value !== 'private_note') {
    return []
  }

  await Promise.all([usersStore.fetchUsers(), teamStore.fetchTeams()])

  const q = query.toLowerCase()

  const users = usersStore.users
    .filter((u) => u.enabled)
    .filter((u) => `${u.first_name} ${u.last_name}`.toLowerCase().includes(q))
    .map((u) => ({
      id: u.id,
      type: 'agent',
      label: `${u.first_name} ${u.last_name}`.trim(),
      avatar_url: u.avatar_url
    }))

  const teams = teamStore.teams
    .filter((t) => t.name.toLowerCase().includes(q))
    .map((t) => ({
      id: t.id,
      type: 'team',
      label: t.name,
      emoji: t.emoji
    }))

  return [...users, ...teams].slice(0, 25)
}

// Handle mentions changed from editor
const handleMentionsChanged = (newMentions) => {
  mentions.value = newMentions
}

const props = defineProps({
  isFullscreen: {
    type: Boolean,
    default: false
  },
  aiPrompts: {
    type: Array,
    required: true
  },
  isSending: {
    type: Boolean,
    required: true
  },
  uploadingFiles: {
    type: Array,
    required: true
  },
  uploadedFiles: {
    type: Array,
    required: false,
    default: () => []
  },
  isDraftLoading: {
    type: Boolean,
    required: false,
    default: false
  }
})

const emit = defineEmits([
  'toggleFullscreen',
  'send',
  'fileUpload',
  'inlineImageUpload',
  'fileDelete',
  'aiPromptSelected'
])

const conversationStore = useConversationStore()
const emitter = useEmitter()
const { t } = useI18n()
const insertContent = ref(null)
const editorRef = ref(null)

// Mode dropdown state. Reply, Forward, and Private Note are all persistent modes.
const canForward = computed(
  () => conversationStore.current?.inbox_channel === 'email'
)

const modeIcon = computed(() => {
  if (messageType.value === 'private_note') return MessageSquareText
  if (messageType.value === 'forward') return MessageSquareShare
  return MessageSquare
})

const modeLabel = computed(() => {
  if (messageType.value === 'private_note') return t('globals.terms.privateNote')
  if (messageType.value === 'forward') return t('globals.terms.forward')
  return t('globals.terms.reply')
})

const editorPlaceholder = computed(() => {
  if (messageType.value === 'forward') return t('replyBox.forwardPlaceholder')
  return t('editor.hint.full')
})

// Live count of forwarded sources still attached to the composer.
const forwardSourceCount = computed(() =>
  messageType.value === 'forward' ? forwardedSources.value.length : 0
)

// Renders one source's HTML for the non-editable preview and at send time.
// Pure helper imported from forwardBody.js — kept out of Tiptap so email HTML survives.
const sourcePreviewHTML = (source) => buildForwardSourceHTML(source)

// Drop a single source from the forward preview when the user clicks its X button.
const removeSource = (uuid) => {
  forwardedSources.value = forwardedSources.value.filter((s) => s.uuid !== uuid)
}

// Tracks the previous mode so we only run forward-entry / forward-exit side-effects once
// when the user actually switches in/out of forward (not on every htmlContent edit).
const previousMessageType = ref(messageType.value)

const setMode = (mode) => {
  if (mode === messageType.value) return
  if (mode === 'forward') {
    const messages = conversationStore.conversationMessages || []
    const eligible = messages.filter(
      (m) =>
        !m.private &&
        m.type !== 'activity' &&
        m.status !== 'pending' &&
        m.status !== 'failed'
    )
    if (eligible.length === 0) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('replyBox.noMessageToForward')
      })
      return
    }
    // Clear recipients (Front pattern) — forward goes to a third party, not back to the contact.
    to.value = ''
    cc.value = ''
    bcc.value = ''
    showCc.value = false
    showBcc.value = false
    // Empty editor for the agent's note. Forwarded sources live in their own state and get
    // concatenated at send time — keeping them out of Tiptap avoids ProseMirror stripping
    // the email HTML during parse.
    htmlContent.value = ''
    forwardedSources.value = eligible.map((m) => ({
      uuid: m.uuid,
      content: m.content,
      meta: m.meta,
      created_at: m.created_at
    }))
  } else if (previousMessageType.value === 'forward') {
    // Leaving forward mode: drop sources and any leftover note.
    htmlContent.value = ''
    forwardedSources.value = []
  }
  previousMessageType.value = mode
  messageType.value = mode
}

const toggleCc = async () => {
  showCc.value = !showCc.value
  await nextTick()
  // If hiding CC field, clear the content and re-validate so stale errors from CC clear.
  if (!showCc.value) {
    cc.value = ''
    await nextTick()
    validateEmails()
  }
}

const toggleBcc = async () => {
  showBcc.value = !showBcc.value
  await nextTick()
  // If hiding BCC field, clear the content and validate email bcc so it doesn't show errors.
  if (!showBcc.value) {
    bcc.value = ''
    await nextTick()
    validateEmails()
  }
}

const toggleFullscreen = () => {
  emit('toggleFullscreen')
}

const enableSend = computed(() => {
  return (
    (textContent.value.trim().length > 0 ||
      conversationStore.getMacro('reply')?.actions?.length > 0 ||
      props.uploadedFiles.length > 0) &&
    emailErrors.value.length === 0 &&
    !props.uploadingFiles.length && !props.isDraftLoading
  )
})

/**
 * Validates email addresses in To, CC, and BCC fields.
 * Populates `emailErrors` with invalid emails grouped by field.
 */
const validateEmails = async () => {
  emailErrors.value = []
  await nextTick()

  const fields = ['to', 'cc', 'bcc']
  const values = { to: to.value, cc: cc.value, bcc: bcc.value }

  fields.forEach((field) => {
    const invalid = values[field]
      .split(',')
      .map((e) => e.trim())
      .filter((e) => e && !validateEmail(e))

    if (invalid.length)
      emailErrors.value.push(`${t('replyBox.invalidEmailsIn')} '${field}': ${invalid.join(', ')}`)
  })
}

/**
 * Send the reply or private note
 */
const handleSend = async () => {
  await validateEmails()
  if (emailErrors.value.length > 0) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('globals.messages.correctEmailErrors')
    })
    return
  }
  emit('send')
}

const handleFileUpload = (event) => {
  emit('fileUpload', event)
}

const handleOnFileDelete = (uuid) => {
  emit('fileDelete', uuid)
}

const handleEmojiSelect = (emoji) => {
  insertContent.value = undefined
  // Force reactivity so the user can select the same emoji multiple times
  nextTick(() => (insertContent.value = emoji))
}

const handleAiPromptSelected = (key) => {
  emit('aiPromptSelected', key)
}

// Watch and update macro view based on message type this filters our macros.
watch(
  messageType,
  (newType) => {
    if (newType === 'reply') {
      macroStore.setCurrentView('replying')
    } else if (newType === 'private_note') {
      macroStore.setCurrentView('adding_private_note')
    }
    // Focus editor on tab change
    setTimeout(() => {
      editorRef.value?.focus()
    }, 50)
  },
  { immediate: true }
)

// Expose focus method for parent components
const focus = () => {
  editorRef.value?.focus()
}
defineExpose({ focus })
</script>
