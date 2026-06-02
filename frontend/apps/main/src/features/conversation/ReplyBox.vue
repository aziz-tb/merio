<!-- frontend/apps/main/src/features/conversation/ReplyBox.vue-->
<template>
  <AlertDialog :open="showContactEmailWarning" @update:open="showContactEmailWarning = $event">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ $t('replyBox.contactEmailMissing') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{
            $t('replyBox.contactEmailMissingDescription', {
              email: conversationStore.current?.contact?.email
            })
          }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="processSend(true, true)">{{
          $t('replyBox.sendAnyway')
        }}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>

  <AlertDialog :open="showMissingTagsWarning" @update:open="showMissingTagsWarning = $event">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ $t('replyBox.missingTagsTitle') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ $t('replyBox.missingTagsDescription') }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ $t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="processSend(false, true)">{{
          $t('replyBox.sendAnyway')
        }}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>

  <Dialog :open="openAIKeyPrompt" @update:open="openAIKeyPrompt = false">
    <DialogContent class="sm:max-w-lg">
      <DialogHeader class="space-y-2">
        <DialogTitle>{{ $t('ai.enterOpenAIAPIKey') }}</DialogTitle>
        <DialogDescription>
          {{
            $t('ai.apiKey.description', {
              provider: 'OpenAI'
            })
          }}
        </DialogDescription>
      </DialogHeader>
      <Form v-slot="{ handleSubmit }" as="" keep-values :validation-schema="formSchema">
        <form id="apiKeyForm" @submit="handleSubmit($event, updateProvider)">
          <FormField v-slot="{ componentField }" name="apiKey">
            <FormItem>
              <FormLabel>{{ $t('globals.terms.apiKey') }}</FormLabel>
              <FormControl>
                <Input type="text" placeholder="sk-am1RLw7XUWGX.." v-bind="componentField" />
              </FormControl>
              <FormMessage />
            </FormItem>
          </FormField>
        </form>
        <DialogFooter>
          <Button
            type="submit"
            form="apiKeyForm"
            :is-loading="isOpenAIKeyUpdating"
            :disabled="isOpenAIKeyUpdating"
          >
            {{ $t('globals.messages.save') }}
          </Button>
        </DialogFooter>
      </Form>
    </DialogContent>
  </Dialog>

  <div class="text-foreground bg-background ">
    <!-- Fullscreen editor -->
    <Dialog :open="isEditorFullscreen" @update:open="isEditorFullscreen = false">
      <DialogContent
        class="max-w-[60%] max-h-[75%] h-[70%] bg-card text-card-foreground p-4 flex flex-col"
        :class="{ '!bg-private': messageType === 'private_note' }"
        @escapeKeyDown="isEditorFullscreen = false"
        :hide-close-button="true"
      >
        <ReplyBoxContent
          v-if="isEditorFullscreen"
          :isFullscreen="true"
          :aiPrompts="aiPrompts"
          :isSending="isSending"
          :isDraftLoading="isDraftLoading"
          :uploadingFiles="uploadingFiles"
          :uploadedFiles="mediaFiles"
          v-model:htmlContent="htmlContent"
          v-model:textContent="textContent"
          v-model:to="to"
          v-model:cc="cc"
          v-model:bcc="bcc"
          v-model:emailErrors="emailErrors"
          v-model:messageType="messageType"
          v-model:selectedInboxId="selectedInboxId"
          v-model:showCc="showCc"
          v-model:showBcc="showBcc"
          v-model:isRecipientsExpanded="isRecipientsExpanded"
          v-model:forwardedSources="forwardedSources"
          v-model:mentions="mentions"
          @toggleFullscreen="isEditorFullscreen = !isEditorFullscreen"
          @send="processSend"
          @fileUpload="handleFileUpload"
          @filesDropped="uploadFiles"
          @fileDelete="handleFileDelete"
          @aiPromptSelected="handleAiPromptSelected"
          @composerOverlayChange="setComposerOverlayOpen"
          @suppressOutsideCollapse="suppressNextOutsideCollapse"
          class="h-full flex-grow"
        />
      </DialogContent>
    </Dialog>

    <!-- Main composer (non-fullscreen). The same wrapper renders both the collapsed
         placeholder and the expanded editor; click-outside / Escape collapse it when
         empty, and clicking anywhere inside expands it. -->
    <div
      ref="composerRoot"
      class="bg-background shadow-[0_0_8px_rgba(0,0,0,0.05)] text-card-foreground border border-border rounded-lg shadow-md overflow-visible mx-4 mb-4 mt-2 flex flex-col transition-all"
      :class="[
        isComposerExpanded ? 'px-2 pt-2' : 'cursor-text',
        { '!bg-private': isComposerExpanded && messageType === 'private_note' }
      ]"
      v-if="!isEditorFullscreen"
      @click="onComposerClick"
      @keydown.esc="onEscape"
    >
      <!-- Collapsed placeholder. Two lines tall, no chrome, no mode dropdown. -->
      <div
        v-if="!isComposerExpanded"
        class="flex h-16 items-center px-4 text-sm text-muted-foreground"
      >
        {{ $t('replyBox.collapsedPlaceholder') }}
      </div>

      <!-- Expanded composer. -->
      <ReplyBoxContent
        v-else
        ref="replyBoxContentRef"
        :isFullscreen="false"
        :aiPrompts="aiPrompts"
        :isSending="isSending"
        :isDraftLoading="isDraftLoading"
        :uploadingFiles="uploadingFiles"
        :uploadedFiles="mediaFiles"
        v-model:htmlContent="htmlContent"
        v-model:textContent="textContent"
        v-model:to="to"
        v-model:cc="cc"
        v-model:bcc="bcc"
        v-model:emailErrors="emailErrors"
        v-model:messageType="messageType"
        v-model:selectedInboxId="selectedInboxId"
        v-model:showCc="showCc"
        v-model:showBcc="showBcc"
        v-model:isRecipientsExpanded="isRecipientsExpanded"
        v-model:forwardedSources="forwardedSources"
        v-model:mentions="mentions"
        @toggleFullscreen="isEditorFullscreen = !isEditorFullscreen"
        @send="processSend"
        @fileUpload="handleFileUpload"
        @filesDropped="uploadFiles"
        @fileDelete="handleFileDelete"
        @aiPromptSelected="handleAiPromptSelected"
        @composerOverlayChange="setComposerOverlayOpen"
        @suppressOutsideCollapse="suppressNextOutsideCollapse"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, watch, computed, toRaw, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useStorage, onClickOutside } from '@vueuse/core'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { MACRO_CONTEXT } from '@main/constants/conversation'
import { INBOX_CHANNEL } from '@main/constants/inbox'
import { useUserStore } from '@main/stores/user'
import { useDraftManager } from '@main/composables/useDraftManager'
import api from '@main/api'
import { useI18n } from 'vue-i18n'
import { useConversationStore } from '@main/stores/conversation'
import { useInboxStore } from '@main/stores/inbox'
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
import { Button } from '@shared-ui/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog'
import { Input } from '@shared-ui/components/ui/input'
import { useEmitter } from '@main/composables/useEmitter'
import { useFileUpload } from '@main/composables/useFileUpload'
import ReplyBoxContent from '@/features/conversation/ReplyBoxContent.vue'
import { buildForwardSourceHTML } from '@/features/conversation/forwardBody.js'
import { UserTypeAgent } from '@/constants/user'
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage
} from '@shared-ui/components/ui/form'
import { toTypedSchema } from '@vee-validate/zod'
import * as z from 'zod'

const formSchema = toTypedSchema(
  z.object({
    apiKey: z.string().min(1, 'API key is required')
  })
)

const { t } = useI18n()
const conversationStore = useConversationStore()
const inboxStore = useInboxStore()
const emitter = useEmitter()
const userStore = useUserStore()

// Setup file upload composable
const {
  uploadingFiles,
  handleFileUpload,
  handleFileDelete,
  uploadFiles,
  mediaFiles,
  clearMediaFiles,
  setMediaFiles
} = useFileUpload({
  linkedModel: 'messages'
})

// Setup draft management composable
const currentDraftKey = computed(() => conversationStore.current?.uuid || null)
const selectedInboxId = ref(conversationStore.current?.inbox_id || null)
const {
  htmlContent,
  textContent,
  isLoading: isDraftLoading,
  clearDraft,
  loadedAttachments,
  loadedMacroActions,
  sendInboxId
} = useDraftManager(currentDraftKey, mediaFiles)

// Rest of existing state
const openAIKeyPrompt = ref(false)
const isOpenAIKeyUpdating = ref(false)
const isEditorFullscreen = ref(false)
const isSending = ref(false)
const messageType = useStorage('replyBoxMessageType', 'reply')
// Forward state isn't persisted across reloads (an empty editor in forward mode would be a bad
// UX). Reset to reply if a previous session left forward mode in localStorage.
if (messageType.value === 'forward') messageType.value = 'reply'
const to = ref('')
const cc = ref('')
const bcc = ref('')
const showCc = ref(false)
const showBcc = ref(false)
const forwardedSources = ref([])
const emailErrors = ref([])
const aiPrompts = ref([])
const composerRoot = ref(null)
const replyBoxContentRef = ref(null)
const showContactEmailWarning = ref(false)
const showMissingTagsWarning = ref(false)
const mentions = ref([])

const isComposerExpanded = ref(false)
const isRecipientsExpanded = ref(false)
const composerOverlayStates = ref({})
const suppressOutsideCollapseUntil = ref(0)
const OUTSIDE_COLLAPSE_SUPPRESSION_MS = 150

const hasComposerOverlayOpen = computed(() =>
  Object.values(composerOverlayStates.value).some(Boolean)
)

const setComposerOverlayOpen = (name, open) => {
  composerOverlayStates.value = {
    ...composerOverlayStates.value,
    [name]: open
  }
}

const clearComposerOverlayStates = () => {
  composerOverlayStates.value = {}
  suppressOutsideCollapseUntil.value = 0
}

const suppressNextOutsideCollapse = () => {
  suppressOutsideCollapseUntil.value = Date.now() + OUTSIDE_COLLAPSE_SUPPRESSION_MS
}

const shouldIgnoreOutsideCollapse = () => {
  if (showContactEmailWarning.value || showMissingTagsWarning.value || openAIKeyPrompt.value) {
    return true
  }
  if (hasComposerOverlayOpen.value) return true
  if (Date.now() <= suppressOutsideCollapseUntil.value) {
    suppressOutsideCollapseUntil.value = 0
    return true
  }
  return false
}

const defaultInboxId = computed(() => conversationStore.current?.inbox_id || null)
const selectedInboxOverrideId = computed(() => {
  if (!selectedInboxId.value || !defaultInboxId.value) return null
  return Number(selectedInboxId.value) !== Number(defaultInboxId.value)
    ? Number(selectedInboxId.value)
    : null
})

const resetSelectedInbox = () => {
  selectedInboxId.value = defaultInboxId.value
}

// "Has work in progress" — any of these keeps the composer expanded
// even when the user clicks outside.
const hasWorkInProgress = computed(() => {
  if (isSending.value) return true
  if (messageType.value === 'forward') return true
  if (forwardedSources.value.length > 0) return true
  if (textContent.value && textContent.value.trim().length > 0) return true
  if (htmlContent.value && htmlContent.value.trim().length > 0) return true
  if (mediaFiles.value.length > 0) return true
  if (uploadingFiles.value.length > 0) return true
  if (conversationStore.getMacro(MACRO_CONTEXT.REPLY)?.actions?.length > 0) return true
  return false
})

const expandComposer = async () => {
  if (isComposerExpanded.value) return
  isComposerExpanded.value = true
  await nextTick()
  replyBoxContentRef.value?.focus?.()
}

const collapseComposer = () => {
  if (!isComposerExpanded.value) return
  if (hasWorkInProgress.value) return
  isComposerExpanded.value = false
  isRecipientsExpanded.value = false
  clearComposerOverlayStates()
  // Reset mode to reply on collapse — the user picks Note / Forward
  // each time they engage, so a stale "private note" doesn't bleed
  // into the next interaction.
  if (messageType.value !== 'reply') {
    messageType.value = 'reply'
  }
}

const onComposerClick = () => {
  if (!isComposerExpanded.value) expandComposer()
}

const onEscape = () => {
  if (hasComposerOverlayOpen.value) return
  collapseComposer()
}

// Click outside the composer root: collapse if there's no work to lose.
onClickOutside(
  composerRoot,
  () => {
    if (shouldIgnoreOutsideCollapse()) return
    collapseComposer()
  },
  {
    ignore: [
      // Reka portals dropdown / select / popover content outside the
      // composer's DOM tree. Treat clicks inside any of these as still
      // "inside" the composer so opening a menu doesn't collapse it.
      '[role="menu"]',
      '[role="listbox"]',
      '[role="dialog"]',
      '[data-radix-popper-content-wrapper]'
    ]
  }
)

// `r` keyboard shortcut: expand the composer if it's idle. Skips when
// the user is already typing into another input / editor so we don't
// hijack the key during normal use. The shortcut is intentionally
// scoped to the document so it works while reading the message thread.
const onGlobalKeydown = (event) => {
  if (event.key !== 'r' && event.key !== 'R') return
  if (event.metaKey || event.ctrlKey || event.altKey) return
  if (isComposerExpanded.value) return

  const active = document.activeElement
  if (active) {
    const tag = active.tagName?.toLowerCase()
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return
    if (active.isContentEditable) return
  }

  event.preventDefault()
  expandComposer()
}

onMounted(() => {
  document.addEventListener('keydown', onGlobalKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onGlobalKeydown)
})

// If work-in-progress appears (e.g. a draft restored, a macro applied
// from the sidebar, forward mode entered) auto-expand so the user
// always sees the work.
watch(hasWorkInProgress, (value) => {
  if (value && !isComposerExpanded.value) {
    expandComposer()
  }
})

watch(isEditorFullscreen, clearComposerOverlayStates)

/**
 * Fetches AI prompts from the server.
 */
const fetchAiPrompts = async () => {
  try {
    const resp = await api.getAiPrompts()
    aiPrompts.value = resp.data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

fetchAiPrompts()

/**
 * Handles the AI prompt selection event.
 * Sends the selected prompt key and the current text content to the server for completion.
 * Sets the response as the new content in the editor.
 * @param {String} key - The key of the selected AI prompt
 */
const handleAiPromptSelected = async (key) => {
  try {
    const resp = await api.aiCompletion({
      prompt_key: key,
      content: textContent.value
    })
    htmlContent.value = resp.data.data.replace(/\n/g, '<br>')
  } catch (error) {
    // Check if user needs to enter OpenAI API key and has permission to do so.
    if (error.response?.status === 400 && userStore.can('ai:manage')) {
      openAIKeyPrompt.value = true
    }
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

/**
 * updateProvider updates the OpenAI API key.
 * @param {Object} values - The form values containing the API key
 */
const updateProvider = async (values) => {
  try {
    isOpenAIKeyUpdating.value = true
    await api.updateAIProvider({ api_key: values.apiKey, provider: 'openai' })
    openAIKeyPrompt.value = false
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isOpenAIKeyUpdating.value = false
  }
}

/**
 * Returns true if the editor has text content.
 */
const hasTextContent = computed(() => {
  return textContent.value.trim().length > 0
})

/**
 * Processes a Forward send. The editor only holds the agent's note. Forwarded sources live
 * in `forwardedSources` (state) and are rendered via `buildForwardSourceHTML` at send time —
 * keeping email HTML out of Tiptap so ProseMirror can't strip it during parse.
 */
const processForward = async () => {
  const convUUID = conversationStore.current.uuid

  if (!to.value.trim()) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('replyBox.toRequired')
    })
    return
  }

  if (forwardedSources.value.length === 0) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('replyBox.noMessageToForward')
    })
    return
  }

  const parseList = (s) =>
    s
      ? s
          .split(',')
          .map((e) => e.trim())
          .filter(Boolean)
      : []

  const note = htmlContent.value || ''
  const blocks = forwardedSources.value.map(buildForwardSourceHTML).join('')
  const body = note + blocks
  const messageUUIDs = forwardedSources.value.map((s) => s.uuid)
  const sendInboxIdToSend = selectedInboxOverrideId.value

  isSending.value = true
  try {
    await api.forwardMessage(convUUID, messageUUIDs[0], {
      to: parseList(to.value),
      cc: parseList(cc.value),
      bcc: parseList(bcc.value),
      message_uuids: messageUUIDs,
      body,
      skip_quote: true,
      ...(sendInboxIdToSend != null && { send_inbox_id: sendInboxIdToSend })
    })
    // Reset state: editor body, recipients, draft, then flip back to reply mode.
    htmlContent.value = ''
    to.value = ''
    cc.value = ''
    bcc.value = ''
    showCc.value = false
    showBcc.value = false
    forwardedSources.value = []
    emailErrors.value = []
    clearDraft(currentDraftKey.value)
    clearMediaFiles()
    messageType.value = 'reply'
    resetSelectedInbox()
    // Collapse after a successful forward — nothing left to edit.
    isComposerExpanded.value = false
    isRecipientsExpanded.value = false
    clearComposerOverlayStates()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isSending.value = false
  }
}

/**
 * Processes the send action.
 */
const processSend = async (skipContactEmailCheck = false, skipMissingTagsCheck = false) => {
  // Forward mode has its own send path: a separate endpoint, no optimistic pending message,
  // attachments cloned server-side from the listed source UUIDs.
  if (messageType.value === 'forward') {
    isEditorFullscreen.value = false
    await processForward()
    return
  }

  let hasMessageSendingErrored = false
  isEditorFullscreen.value = false

  const hasContent = hasTextContent.value > 0 || mediaFiles.value.length > 0
  const convUUID = conversationStore.current.uuid
  const isPrivate = messageType.value === 'private_note'
  const sendInboxIdToSend = !isPrivate ? selectedInboxOverrideId.value : null

  const currentInbox = inboxStore.inboxes.find(
    (i) => i.id === conversationStore.current.inbox_id
  )
  if (
    !isPrivate &&
    !skipMissingTagsCheck &&
    currentInbox?.prompt_tags_on_reply &&
    !(conversationStore.current.tags?.length > 0)
  ) {
    showMissingTagsWarning.value = true
    return
  }

  if (!isPrivate && conversationStore.current.inbox_channel === 'email') {
    // Require at least one recipient in `to`.
    if (!to.value.trim()) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('replyBox.toRequired')
      })
      return
    }

    // Warn if the contact's email is not in any recipient field.
    if (!skipContactEmailCheck) {
      const contactEmail = conversationStore.current.contact?.email?.toLowerCase()
      if (contactEmail) {
        const allRecipients = [to.value, cc.value, bcc.value].join(',').toLowerCase()
        if (
          !allRecipients
            .split(',')
            .map((e) => e.trim())
            .includes(contactEmail)
        ) {
          showContactEmailWarning.value = true
          return
        }
      }
    }
  }
  let tempUUID = null

  // Add pending message to cache for instant display.
  if (hasContent) {
    const savedContent = htmlContent.value
    const author = {
      id: userStore.userID,
      first_name: userStore.firstName,
      last_name: userStore.lastName,
      avatar_url: userStore.avatar,
      type: 'agent'
    }
    const parsedTo =
      !isPrivate && to.value
        ? to.value
            .split(',')
            .map((e) => e.trim())
            .filter(Boolean)
        : []
    const parsedCC =
      !isPrivate && cc.value
        ? cc.value
            .split(',')
            .map((e) => e.trim())
            .filter(Boolean)
        : []
    const parsedBCC =
      !isPrivate && bcc.value
        ? bcc.value
            .split(',')
            .map((e) => e.trim())
            .filter(Boolean)
        : []
    const meta = {}
    if (parsedTo.length) meta.to = parsedTo
    if (parsedCC.length) meta.cc = parsedCC
    if (parsedBCC.length) meta.bcc = parsedBCC

    tempUUID = conversationStore.addPendingMessage(
      convUUID,
      savedContent,
      isPrivate,
      author,
      mediaFiles.value,
      textContent.value,
      meta
    )

    // Clear editor immediately.
    htmlContent.value = ''

    try {
      isSending.value = true
      const response = await api.sendMessage(convUUID, {
        sender_type: UserTypeAgent,
        private: isPrivate,
        message: savedContent,
        attachments: mediaFiles.value.map((file) => file.id),
        mentions: isPrivate ? mentions.value : [],
        cc: parsedCC,
        bcc: parsedBCC,
        to: parsedTo,
        echo_id: isPrivate ? '' : tempUUID,
        ...(sendInboxIdToSend != null && { send_inbox_id: sendInboxIdToSend })
      })

      // Private notes are sent immediately so replace immediately.
      if (isPrivate && response?.data?.data) {
        conversationStore.replacePendingMessage(convUUID, tempUUID, response.data.data)
      }
    } catch (error) {
      hasMessageSendingErrored = true
      // Remove pending message and restore editor content.
      conversationStore.removePendingMessage(convUUID, tempUUID)
      htmlContent.value = savedContent
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    }
  }

  // Apply macro actions if any.
  if (!hasMessageSendingErrored) {
    const macroID = conversationStore.getMacro(MACRO_CONTEXT.REPLY)?.id
    const macroActions = conversationStore.getMacro(MACRO_CONTEXT.REPLY)?.actions || []
    if (macroID > 0 && macroActions.length > 0) {
      try {
        await api.applyMacro(convUUID, macroID, macroActions)
      } catch (error) {
        emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
          variant: 'destructive',
          description: handleHTTPError(error).message
        })
      }
    }
  }

  // Clear state on success and collapse the composer back to its idle
  // placeholder. Mode resets to reply so a private note doesn't bleed
  // into the next interaction.
  if (!hasMessageSendingErrored) {
    clearDraft(currentDraftKey.value)
    conversationStore.resetMacro(MACRO_CONTEXT.REPLY)
    clearMediaFiles()
    emailErrors.value = []
    mentions.value = []
    messageType.value = 'reply'
    resetSelectedInbox()
    isComposerExpanded.value = false
    isRecipientsExpanded.value = false
    clearComposerOverlayStates()
  }
  isSending.value = false
}

/**
 * Watches for changes in the conversation's macro id and update message content.
 */
watch(
  () => conversationStore.getMacro(MACRO_CONTEXT.REPLY).id,
  (newId) => {
    // No macro set.
    if (!newId) return

    const macro = conversationStore.getMacro(MACRO_CONTEXT.REPLY)
    if (macro?._inserted_from_editor) return

    // If macro has message content, set it in the editor.
    if (macro.message_content) {
      htmlContent.value = macro.message_content
    }
  },
  { deep: true }
)

/**
 * Watch loaded macro actions from draft and update conversation store.
 */
watch(
  loadedMacroActions,
  (actions) => {
    if (actions.length > 0) {
      conversationStore.setMacroActions([...toRaw(actions)], MACRO_CONTEXT.REPLY)
    }
  },
  { deep: true }
)

/**
 * Watch for loaded attachments from draft and restore them to mediaFiles.
 */
watch(
  loadedAttachments,
  (attachments) => {
    if (attachments.length > 0) {
      setMediaFiles([...attachments])
    }
  },
  { deep: true }
)

watch(
  sendInboxId,
  (newVal) => {
    selectedInboxId.value = newVal != null ? Number(newVal) : defaultInboxId.value
  },
  { immediate: true }
)

watch(
  selectedInboxId,
  (newVal, oldVal) => {
    if (!newVal || Number(newVal) === Number(oldVal)) return
    if (conversationStore.current?.inbox_channel === INBOX_CHANNEL.EMAIL) {
      conversationStore.recomputeRecipientsForInbox(newVal)
    }
    sendInboxId.value = selectedInboxOverrideId.value
  }
)

// Initialize to, cc, and bcc fields with the current conversation's values.
watch(
  () => conversationStore.currentCC,
  (newVal) => {
    const newCc = newVal?.join(', ') || ''
    cc.value = newCc
    // Auto-show the CC field when there's existing CC content (e.g. inherited from the conversation).
    if (newCc.length > 0) {
      showCc.value = true
    }
  },
  { deep: true, immediate: true }
)

watch(
  () => conversationStore.currentTo,
  (newVal) => {
    to.value = newVal?.join(', ') || ''
  },
  { immediate: true }
)

watch(
  () => conversationStore.currentBCC,
  (newVal) => {
    const newBcc = newVal?.join(', ') || ''
    bcc.value = newBcc
    // Only show BCC field if it has content
    if (newBcc.length > 0) {
      showBcc.value = true
    }
  },
  { deep: true, immediate: true }
)

// Clear media files and reset macro when conversation changes.
watch(
  () => conversationStore.current?.uuid,
  () => {
    clearMediaFiles()
    conversationStore.resetMacro(MACRO_CONTEXT.REPLY)
    resetSelectedInbox()
    // Forward state is conversation-scoped: dropping it on switch avoids stale quoted blocks
    // from conversation A leaking into conversation B's composer.
    if (messageType.value === 'forward') {
      messageType.value = 'reply'
      htmlContent.value = ''
      to.value = ''
      cc.value = ''
      bcc.value = ''
      showCc.value = false
      showBcc.value = false
      forwardedSources.value = []
    }
    // Collapse on conversation switch so the new conversation starts
    // idle. If a draft exists for the new conversation the
    // hasWorkInProgress watcher above will auto-expand.
    isComposerExpanded.value = false
    isRecipientsExpanded.value = false
    clearComposerOverlayStates()
  }
)
</script>
