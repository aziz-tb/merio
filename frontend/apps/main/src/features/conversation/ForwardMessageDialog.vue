<!-- frontend/apps/main/src/features/conversation/ForwardMessageDialog.vue
     Forward an existing message to one or more new recipients. v1: comma-separated text input
     for To/Cc/Bcc (autocomplete deferred to v2), optional comment prepended above the quoted body.
     Original attachments are auto-cloned by the backend; the dialog only collects recipients + comment. -->
<template>
  <Dialog :open="open" @update:open="(v) => emit('update:open', v)">
    <DialogContent class="sm:max-w-2xl max-h-[85vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>Forward message</DialogTitle>
        <DialogDescription>
          Send this message to one or more new recipients. The original message and its attachments
          will be quoted below.
        </DialogDescription>
      </DialogHeader>

      <div class="space-y-3">
        <div class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">TO:</label>
          <Input
            type="text"
            placeholder="email@example.com, ..."
            v-model="to"
            class="flex-grow"
          />
        </div>
        <div class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">CC:</label>
          <Input type="text" placeholder="(optional)" v-model="cc" class="flex-grow" />
          <Button
            size="sm"
            variant="ghost"
            @click="showBcc = !showBcc"
            class="text-xs"
          >
            {{ showBcc ? 'Hide BCC' : 'Add BCC' }}
          </Button>
        </div>
        <div v-if="showBcc" class="flex items-center space-x-2">
          <label class="w-12 text-sm font-medium text-muted-foreground">BCC:</label>
          <Input type="text" placeholder="(optional)" v-model="bcc" class="flex-grow" />
        </div>

        <div>
          <label class="text-sm font-medium text-muted-foreground">Comment (optional)</label>
          <Textarea
            v-model="comment"
            placeholder="Add a note for the recipients above the forwarded message…"
            rows="3"
            class="mt-1"
          />
        </div>

        <!-- Read-only preview of the original -->
        <div class="border border-border rounded-md p-3 text-xs space-y-1 bg-muted/30">
          <p class="text-muted-foreground">Original message</p>
          <p v-if="origSubject" class="text-foreground"><b>Subject:</b> {{ origSubject }}</p>
          <p v-if="origFrom"><b>From:</b> {{ origFrom }}</p>
          <p v-if="origDate"><b>Date:</b> {{ origDate }}</p>
          <p v-if="origTo"><b>To:</b> {{ origTo }}</p>
          <div v-if="message?.content" class="mt-2 max-h-40 overflow-y-auto border-t border-border pt-2">
            <Letter :html="message.content" :allowedSchemas="['cid', 'https', 'http', 'mailto']" />
          </div>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="emit('update:open', false)" :disabled="isSending">
          Cancel
        </Button>
        <Button @click="handleSend" :disabled="isSending || !to.trim()" :is-loading="isSending">
          Forward
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { Letter } from 'vue-letter'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared-ui/components/ui/dialog'
import { Input } from '@shared-ui/components/ui/input'
import { Textarea } from '@shared-ui/components/ui/textarea'
import { Button } from '@shared-ui/components/ui/button'
import { useConversationStore } from '@main/stores/conversation'
import { useEmitter } from '@main/composables/useEmitter'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { formatFullTimestamp } from '@shared-ui/utils/datetime.js'
import api from '@main/api'

const props = defineProps({
  open: { type: Boolean, default: false },
  message: { type: Object, required: true }
})
const emit = defineEmits(['update:open'])

const conversationStore = useConversationStore()
const emitter = useEmitter()

const to = ref('')
const cc = ref('')
const bcc = ref('')
const showBcc = ref(false)
const comment = ref('')
const isSending = ref(false)

// Reset fields whenever the dialog opens.
watch(() => props.open, (open) => {
  if (open) {
    to.value = ''
    cc.value = ''
    bcc.value = ''
    showBcc.value = false
    comment.value = ''
  }
})

const origSubject = computed(() => props.message?.meta?.subject || '')
const origFrom = computed(() => (props.message?.meta?.from || []).join(', '))
const origTo = computed(() => (props.message?.meta?.to || []).join(', '))
const origDate = computed(() =>
  props.message?.created_at ? formatFullTimestamp(props.message.created_at) : ''
)

const parseList = (s) =>
  (s || '').split(',').map((x) => x.trim()).filter(Boolean)

const handleSend = async () => {
  const convUUID = conversationStore.current?.uuid
  if (!convUUID || !props.message?.uuid) return
  isSending.value = true
  try {
    await api.forwardMessage(convUUID, props.message.uuid, {
      to: parseList(to.value),
      cc: parseList(cc.value),
      bcc: parseList(bcc.value),
      comment: comment.value
    })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: 'Message forwarded.' })
    emit('update:open', false)
  } catch (err) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(err).message
    })
  } finally {
    isSending.value = false
  }
}
</script>
