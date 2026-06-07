<!-- frontend/apps/main/src/features/conversation/ReplyBoxContent.vue -->
<template>
  <!-- Set fixed width only when not in fullscreen. -->
  <div class="flex flex-col h-full" :class="{ 'max-h-[600px]': !isFullscreen }">
    <!-- Top row: mode dropdown (reply / forward / private note) on the left,
         collapsed recipients summary inline next to it (email channels only),
         fullscreen toggle on the right. -->
    <div
      class="flex justify-between items-center"
      :class="{ 'mb-1': !isFullscreen, 'border-b border-border pb-2': isFullscreen }"
    >
      <div class="flex items-center gap-3 min-w-0">
        <DropdownMenu @update:open="(open) => emitComposerOverlayChange('mode-menu', open)">
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="inline-flex  items-center gap-1.5 p-1 rounded text-sm font-semibold hover:bg-accent transition-colors"
              :class="messageType === 'private_note' ? 'text-amber-700' : 'text-foreground'"
            >
              <component :is="modeIcon" class="size-4" />
              <span>{{ modeLabel }}</span>
              <ChevronDown class="size-4 opacity-60" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            class="w-44"
            @pointerDownOutside="emitSuppressOutsideCollapse"
          >
            <DropdownMenuItem @click="setMode('reply')">
              <Message class="size-4 mr-2" />
              <span>{{ $t('globals.terms.reply') }}</span>
            </DropdownMenuItem>
            <DropdownMenuItem v-if="canForward" @click="setMode('forward')">
              <MessageArrowUpRight class="size-4 mr-2" />
              <span>{{ $t('globals.terms.forward') }}</span>
            </DropdownMenuItem>
            <DropdownMenuItem @click="setMode('private_note')">
              <MessageLines class="size-4 mr-2" />
              <span>{{ $t('globals.terms.privateNote') }}</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <!-- Collapsed recipients summary. Only renders when recipients are
             not expanded AND this is an email-channel reply or forward. -->
        <button
          v-if="showRecipientsSummary"
          type="button"
          class="inline-flex min-w-0 items-center truncate text-sm text-muted-foreground hover:text-blue-500 transition-colors"
          :title="recipientsSummaryTitle"
          @click.stop="expandRecipients"
        >
          <span class="truncate">{{ recipientsSummary }}</span>
        </button>

        <DropdownMenu
          v-if="showInboxSelector && !isRecipientsExpanded"
          @update:open="(open) => emitComposerOverlayChange('compact-inbox-menu', open)"
        >
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="inline-flex min-w-0 items-center truncate text-sm text-muted-foreground hover:text-blue-500 transition-colors"
              :title="selectedInbox?.from || ''"
            >
              <span class="truncate">{{ selectedInbox?.name || $t('replyBox.selectInbox') }}</span>
              <ChevronDown class="size-3.5 ml-1 opacity-60" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            class="w-72"
            @pointerDownOutside="emitSuppressOutsideCollapse"
          >
            <DropdownMenuItem
              v-for="inbox in availableSendInboxes"
              :key="inbox.id"
              @click="selectedInboxId = inbox.id"
            >
              <div class="flex flex-col">
                <span class="text-sm font-medium">{{ inbox.name }}</span>
                <span class="text-xs text-muted-foreground">{{ inbox.from }}</span>
              </div>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <!-- Forward status chip: live count of forwarded sources still in the editor. -->
        <span
          v-if="messageType === 'forward' && forwardSourceCount > 0"
          class="inline-flex items-center text-xs text-muted-foreground bg-muted/50 rounded px-2 py-0.5"
        >
          {{ $t('replyBox.forwardingMessages', forwardSourceCount, { count: forwardSourceCount }) }}
        </span>
      </div>
      <Button class="text-muted-foreground" variant="ghost" size="icon" @click="toggleFullscreen">
        <component :is="isFullscreen ? Minimize2 : Maximize2" class="size-4.5" />
      </Button>
    </div>

    <!-- TO, CC, and BCC inputs. Visible in reply + forward modes (email channel only)
         when recipients are expanded. Otherwise the collapsed summary above shows them. -->
    <div v-if="conversationStore.current.inbox_channel === INBOX_CHANNEL.EMAIL && isRecipientsExpanded">
      <div
        :class="['space-y-3', isFullscreen ? 'p-4 border-b border-border' : 'mb-4']"
        v-if="messageType === 'reply' || messageType === 'forward'"
      >
        <div v-if="showInboxSelector" class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">{{ $t('replyBox.fromLabel') }}:</label>
          <Select
            :model-value="selectedInboxId ? String(selectedInboxId) : ''"
            @update:model-value="(v) => selectedInboxId = v ? Number(v) : null"
            @update:open="(open) => emitComposerOverlayChange('from-select', open)"
          >
            <SelectTrigger class="flex-grow">
              <SelectValue :placeholder="$t('replyBox.selectInbox')" />
            </SelectTrigger>
            <SelectContent @pointerDownOutside="emitSuppressOutsideCollapse">
              <SelectItem
                v-for="inbox in availableSendInboxes"
                :key="inbox.id"
                :value="String(inbox.id)"
              >
                {{ inbox.name }} ({{ inbox.from }})
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
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
        :getMacros="getMacros"
        :onMacroPicked="onMacroPicked"
        @aiPromptSelected="handleAiPromptSelected"
        @send="handleSend"
        @mentionsChanged="handleMentionsChanged"
        @files-dropped="handleFilesDropped"
        @composerOverlayChange="emitComposerOverlayChange"
        @suppressOutsideCollapse="emitSuppressOutsideCollapse"
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
          <X class="size-4" />
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
      @composerOverlayChange="emitComposerOverlayChange"
      @suppressOutsideCollapse="emitSuppressOutsideCollapse"
    />
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch } from 'vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { MACRO_CONTEXT } from '@main/constants/conversation'
import { INBOX_CHANNEL } from '@main/constants/inbox'
import { ArrowUpRightAndArrowDownLeftFromCenter as Maximize2, ArrowDownLeftAndArrowUpRightToCenter as Minimize2, ChevronDown, X, Message, MessageArrowUpRight, MessageLines } from '@qall/icons'
import Editor from '@main/components/editor/TextEditor.vue'
import { useConversationStore } from '@main/stores/conversation'
import { useInboxStore } from '@main/stores/inbox'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
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
const selectedInboxId = defineModel('selectedInboxId', { default: null })
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

// Recipients expanded state — when false, the TO/CC/BCC inputs are hidden
// and a compact summary is shown next to the mode dropdown. Owned by the
// parent ReplyBox so it can collapse on click-outside / Escape.
const isRecipientsExpanded = defineModel('isRecipientsExpanded', { default: false })

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

const getMacros = async (query) => {
  if (messageType.value === 'forward') return []

  await macroStore.loadMacros()

  const q = query.toLowerCase()
  const filtered = macroStore.macroOptions.filter((m) =>
    (m.name || '').toLowerCase().includes(q)
  )

  return [...filtered].sort((a, b) => (b.usage_count || 0) - (a.usage_count || 0))
}

const onMacroPicked = (macro) => {
  const plainMacro = JSON.parse(JSON.stringify(macro))
  plainMacro._inserted_from_editor = true
  conversationStore.setMacro(plainMacro, MACRO_CONTEXT.REPLY)
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
  'aiPromptSelected',
  'filesDropped',
  'composerOverlayChange',
  'suppressOutsideCollapse'
])

const conversationStore = useConversationStore()
const inboxStore = useInboxStore()
const emitter = useEmitter()
const { t } = useI18n()
const insertContent = ref(null)
const editorRef = ref(null)

const emitComposerOverlayChange = (name, open) => {
  emit('composerOverlayChange', name, open)
}

const emitSuppressOutsideCollapse = () => {
  emit('suppressOutsideCollapse')
}

// Mode dropdown state. Reply, Forward, and Private Note are all persistent modes.
const canForward = computed(
  () => conversationStore.current?.inbox_channel === INBOX_CHANNEL.EMAIL
)

const modeIcon = computed(() => {
  if (messageType.value === 'private_note') return MessageLines
  if (messageType.value === 'forward') return MessageArrowUpRight
  return Message
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

const availableSendInboxes = computed(() => inboxStore.emailInboxesEnabled)

const showInboxSelector = computed(() => {
  if (conversationStore.current?.inbox_channel !== INBOX_CHANNEL.EMAIL) return false
  if (messageType.value !== 'reply' && messageType.value !== 'forward') return false
  return availableSendInboxes.value.length >= 2
})

const selectedInbox = computed(() =>
  availableSendInboxes.value.find((inbox) => Number(inbox.id) === Number(selectedInboxId.value))
)

// Renders one source's HTML for the non-editable preview and at send time.
// Pure helper imported from forwardBody.js — kept out of Tiptap so email HTML survives.
const sourcePreviewHTML = (source) => buildForwardSourceHTML(source)

// Drop a single source from the forward preview when the user clicks its X button.
const removeSource = (uuid) => {
  forwardedSources.value = forwardedSources.value.filter((s) => s.uuid !== uuid)
}

// ──────────────────────────────────────────────────────────────────
// Collapsed recipients summary. Renders next to the mode dropdown
// when recipients aren't expanded. Shows "To: x@y.com +N" where N is
// the total of all other recipients across TO, CC, BCC.
// ──────────────────────────────────────────────────────────────────
const parseList = (s) =>
  s
    ? s
        .split(',')
        .map((e) => e.trim())
        .filter(Boolean)
    : []

const toList = computed(() => parseList(to.value))
const ccList = computed(() => parseList(cc.value))
const bccList = computed(() => parseList(bcc.value))

const allRecipientsCount = computed(
  () => toList.value.length + ccList.value.length + bccList.value.length
)

const showRecipientsSummary = computed(() => {
  if (conversationStore.current?.inbox_channel !== INBOX_CHANNEL.EMAIL) return false
  if (messageType.value !== 'reply' && messageType.value !== 'forward') return false
  if (isRecipientsExpanded.value) return false
  return true
})

const recipientsSummary = computed(() => {
  const first = toList.value[0]
  if (!first) {
    return t('replyBox.addRecipient')
  }
  const remaining = allRecipientsCount.value - 1
  if (remaining <= 0) return `${first}`
  return `${first} +${remaining}`
})

const recipientsSummaryTitle = computed(() => {
  const parts = []
  if (toList.value.length) parts.push(`To: ${toList.value.join(', ')}`)
  if (ccList.value.length) parts.push(`Cc: ${ccList.value.join(', ')}`)
  if (bccList.value.length) parts.push(`Bcc: ${bccList.value.join(', ')}`)
  return parts.join(' · ') || t('replyBox.addRecipient')
})

const expandRecipients = async () => {
  isRecipientsExpanded.value = true
  await nextTick()
  // Focus the editor again rather than yanking focus into the TO input.
  // The user clicked the summary to see the inputs, not necessarily to
  // start editing the recipient list — they can click into the input
  // themselves if needed.
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
    // Forward starts with recipients expanded — the user always needs to
    // fill in the new TO, so showing the input immediately is friendlier
    // than making them click the summary first.
    isRecipientsExpanded.value = true
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

const handleFilesDropped = (files) => {
  emit('filesDropped', files)
}

const handleOnFileDelete = (uuid) => {
  emit('fileDelete', uuid)
}

const handleEmojiSelect = (emoji) => {
  insertContent.value = undefined
  // Force reactivity so the user can select the same emoji multiple times
  nextTick(() => (insertContent.value = emoji))
}

const handleAiPromptSelected = (payload) => {
  if (typeof payload === 'string') {
    emit('aiPromptSelected', {
      key: payload,
      content: textContent.value,
      html: htmlContent.value
    })
    return
  }
  emit('aiPromptSelected', payload)
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
