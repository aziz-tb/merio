<template>
  <!-- Set fixed width only when not in fullscreen. -->
  <div class="flex flex-col h-full" :class="{ 'max-h-[600px]': !isFullscreen }">
    <!-- Top action bar — labelled private-note toggle + fullscreen.
         The previous tab strip has been replaced by a single labelled button: clicking toggles
         `messageType` between 'reply' and 'private_note'. Active (private-note) state gets an
         amber-50 fill + amber-700 text + amber-200 border so the control itself signals the mode,
         alongside the editor-wide `!bg-private` tint that already exists in ReplyBox.vue. -->
    <div
      class="flex justify-between items-center"
      :class="{ 'mb-2': !isFullscreen, 'border-b border-border pb-2': isFullscreen }"
    >
      <Button
        variant="ghost"
        size="sm"
        :class="messageType === 'private_note'
          ? 'bg-amber-50 text-amber-700 hover:bg-amber-100 hover:text-amber-700 border border-amber-200'
          : 'text-muted-foreground hover:text-foreground'"
        @click="messageType = messageType === 'private_note' ? 'reply' : 'private_note'"
      >
        <StickyNote class="size-4 mr-1.5" />
        {{ $t('globals.terms.privateNote') }}
      </Button>
      <Button class="text-muted-foreground" variant="ghost" size="icon" @click="toggleFullscreen">
        <component :is="isFullscreen ? Minimize2 : Maximize2" class="size-4" />
      </Button>
    </div>

    <!-- To, CC, and BCC fields -->
    <div v-if="conversationStore.current.inbox_channel === 'email'">
      <div
        :class="['space-y-3', isFullscreen ? 'p-4 border-b border-border' : 'mb-4']"
        v-if="messageType === 'reply'"
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
        </div>
        <div class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">CC:</label>
          <Input
            type="text"
            :placeholder="t('replyBox.emailAddresess')"
            v-model="cc"
            class="flex-grow px-3 py-2 text-sm border rounded focus:ring-2 focus:ring-ring"
            @blur="validateEmails"
          />
          <Button
            size="sm"
            @click="toggleBcc"
            class="text-sm bg-secondary text-secondary-foreground hover:bg-secondary/80"
          >
            {{ showBcc ? $t('replyBox.removeBCC') : $t('replyBox.bcc') }}
          </Button>
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
        :placeholder="t('editor.hint.full')"
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

    <!-- Editor menu bar with send button -->
    <ReplyBoxMenuBar
      class="mt-1 shrink-0"
      :isFullscreen="isFullscreen"
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
import { Maximize2, Minimize2, StickyNote } from 'lucide-vue-next'
import Editor from '@main/components/editor/TextEditor.vue'
import { useConversationStore } from '@main/stores/conversation'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button'
import { useEmitter } from '@main/composables/useEmitter'
import AttachmentsPreview from '@/features/conversation/message/attachment/AttachmentsPreview.vue'
import MacroActionsPreview from '@/features/conversation/MacroActionsPreview.vue'
import ReplyBoxMenuBar from '@/features/conversation/ReplyBoxMenuBar.vue'
import { useI18n } from 'vue-i18n'
import { validateEmail } from '@shared-ui/utils/string'
import { useMacroStore } from '@main/stores/macro'
import { useUsersStore } from '@main/stores/users'
import { useTeamStore } from '@main/stores/team'

const messageType = defineModel('messageType', { default: 'reply' })
const to = defineModel('to', { default: '' })
const cc = defineModel('cc', { default: '' })
const bcc = defineModel('bcc', { default: '' })
const showBcc = defineModel('showBcc', { default: false })
const emailErrors = defineModel('emailErrors', { default: () => [] })
const htmlContent = defineModel('htmlContent', { default: '' })
const textContent = defineModel('textContent', { default: '' })
const mentions = defineModel('mentions', { default: () => [] })
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
  (newType, oldType) => {
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
