// useAgentPreview — right-rail Preview panel state for the Edit Agent
// layout.
//
// Owned by the route-scoped layout provider above the AI editor so
// state survives tab switches and can be shared with any sibling UI
// (such as the global sidebar). Step 10 swaps the local `isOpen` ref
// for a localStorage-backed one.
//
// Per fork-4 resolution, `from_source` is NOT a UI state — the From
// dropdown's label is derived from `linkedTicket`:
//   - linkedTicket === null → "Preview user"
//   - linkedTicket !== null → `#<id> · <contact name>`
// Customer / Contract items in the dropdown are static disabled
// options with no state to track until Phase 2.
//
// Run payload uses the existing /ai/agents/:id/run shape:
//   { inbound_message, conversation_id, verification_level, ... }
// Per the fork resolution, `safe_mode` is sent through (backend
// accepts but doesn't yet enforce — separate batch).

import { computed, ref, shallowRef, toValue, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import api from '@/api'
import { useEmitter } from '@/composables/useEmitter'
import { useAgentRunStream } from '@/composables/useAgentRunStream'
import { useAgentRightRail } from '@/composables/useAgentRightRail'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'

export const AGENT_PREVIEW_KEY = Symbol('AgentPreview')

export function useAgentPreview(agentId) {
  const emitter = useEmitter()
  const { t } = useI18n()
  const rail = useAgentRightRail()
  const resolveAgentID = () => Number(toValue(agentId))

  // Persisted preference: whether the preview should claim the rail on page
  // load. The actual open state is coordinated by useAgentRightRail so other
  // panels, such as the path picker, can temporarily claim the same slot.
  const preferOpen = useStorage('merio:agent-preview-prefer-open', true)
  const isOpen = computed(() => rail.isActive('preview'))

  // Channel — 'chat' | 'email'. Email is the v1 Merio default;
  // Chat is rendered but the run payload doesn't distinguish today.
  const channel = ref('email')

  // Linked ticket (loaded via the "From ticket…" popover in step 8).
  // Null → Preview user mode.
  const linkedTicket = ref(null)

  // Compose fields. Subject is email-only; ignored for chat.
  const subject = ref('')
  const composeBody = ref('')

  // Safe mode — sent to the backend, enforced once the tool category
  // split ships.
  const safeMode = ref(false)

  // Per-run verification override. Defaults to whatever was set when
  // the ticket was loaded; 0 (public) otherwise.
  const verificationOverride = ref(0)

  const lastRun = ref(null)
  const running = ref(false)
  const activeStream = shallowRef(null)
  let stopStreamWatch = null

  const streamEvents = computed(() => activeStream.value?.events.value || [])
  const streamStatus = computed(() => activeStream.value?.status.value || 'idle')
  const streamError = computed(() => activeStream.value?.error.value || '')

  const clearStream = () => {
    if (stopStreamWatch) {
      stopStreamWatch()
      stopStreamWatch = null
    }
    if (activeStream.value) {
      activeStream.value.disconnect()
      activeStream.value = null
    }
  }

  function loadFromTicket(ticket) {
    if (!ticket) return
    linkedTicket.value = ticket
    subject.value = ticket.subject ?? ''
    composeBody.value = ticket.last_message_text ?? ticket.body ?? ''
    verificationOverride.value = ticket.verification ?? 0
  }

  function reset() {
    linkedTicket.value = null
    subject.value = ''
    composeBody.value = ''
    verificationOverride.value = 0
  }

  function resetSession() {
    clearStream()
    channel.value = 'email'
    safeMode.value = false
    lastRun.value = null
    running.value = false
    reset()
  }

  function open() {
    rail.open('preview')
    preferOpen.value = true
  }

  function close() {
    rail.close()
    preferOpen.value = false
  }

  function toggle() {
    if (isOpen.value) close()
    else open()
  }

  async function run() {
    const id = resolveAgentID()
    if (!composeBody.value.trim() || running.value || !Number.isFinite(id) || id <= 0) return
    running.value = true
    lastRun.value = null
    try {
      const body = {
        inbound_message: composeBody.value,
        conversation_id: linkedTicket.value?.id ?? 0,
        verification_level: verificationOverride.value,
        safe_mode: safeMode.value,
      }
      const resp = await api.runAIAgent(id, body)
      const accepted = resp.data?.data ?? resp.data
      if (!accepted?.trace_id) {
        throw new Error('missing trace_id')
      }

      clearStream()
      const stream = useAgentRunStream(accepted.trace_id)
      activeStream.value = stream
      stopStreamWatch = watch(
        [stream.status, stream.finalResult, stream.error],
        ([nextStatus, nextResult, nextError]) => {
          if (nextStatus === 'completed') {
            lastRun.value = nextResult
            running.value = false
            clearStream()
            return
          }
          if (nextStatus === 'failed') {
            running.value = false
            lastRun.value = null
            emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
              variant: 'destructive',
              description: nextError || t('globals.messages.somethingWentWrong'),
            })
          }
        }
      )
      stream.connect()
    } catch (err) {
      lastRun.value = null
      running.value = false
      clearStream()
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(err).message,
      })
    }
  }

  return {
    isOpen,
    preferOpen,
    channel,
    linkedTicket,
    subject,
    composeBody,
    safeMode,
    verificationOverride,
    lastRun,
    running,
    streamEvents,
    streamStatus,
    streamError,
    open,
    close,
    toggle,
    run,
    loadFromTicket,
    reset,
    resetSession,
  }
}
