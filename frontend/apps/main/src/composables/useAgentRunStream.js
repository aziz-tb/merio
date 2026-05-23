import { ref } from 'vue'
import { normalizeRunResponse } from '@/features/ai/shared/normalizeRunResult.js'

const terminalStatuses = new Set(['completed', 'failed'])
const replayableEventTypes = [
  'run.enqueued',
  'run.started',
  'turn.started',
  'tool.dispatched',
  'tool.completed',
  'draft.committed',
  'run.completed',
  'run.failed'
]

export function useAgentRunStream(traceId) {
  const events = ref([])
  const status = ref('idle')
  const finalResult = ref(null)
  const executionId = ref(null)
  const error = ref('')

  let source = null
  let queuedTimer = null
  let sawRunStarted = false
  let sawTurnActivity = false

  const clearQueuedTimer = () => {
    if (queuedTimer) {
      clearTimeout(queuedTimer)
      queuedTimer = null
    }
  }

  const closeSource = () => {
    if (source) {
      source.close()
      source = null
    }
  }

  const setNonTerminalStatus = (next) => {
    if (!terminalStatuses.has(status.value)) {
      status.value = next
    }
  }

  const pushEvent = (streamEvent, raw) => {
    if (streamEvent.type === 'turn.completed') {
      return
    }
    events.value = [
      ...events.value,
      {
        id: raw?.lastEventId || '',
        type: streamEvent.type,
        emitted_at: streamEvent.emitted_at,
        payload: streamEvent.payload || {},
      }
    ]
  }

  const handleEvent = (streamEvent, raw) => {
    switch (streamEvent.type) {
      case 'run.enqueued':
        clearQueuedTimer()
        queuedTimer = setTimeout(() => {
          if (!sawRunStarted && !terminalStatuses.has(status.value)) {
            status.value = 'queued'
          }
        }, 10000)
        pushEvent(streamEvent, raw)
        return

      case 'run.started':
        sawRunStarted = true
        clearQueuedTimer()
        setNonTerminalStatus('starting')
        pushEvent(streamEvent, raw)
        return

      case 'turn.started':
      case 'tool.dispatched':
      case 'tool.completed':
      case 'draft.committed':
        sawTurnActivity = true
        clearQueuedTimer()
        setNonTerminalStatus('running')
        pushEvent(streamEvent, raw)
        return

      case 'turn.completed':
        clearQueuedTimer()
        return

      case 'run.completed':
        clearQueuedTimer()
        executionId.value = streamEvent.payload?.execution_id || null
        finalResult.value = normalizeRunResponse(streamEvent.payload?.response || null)
        status.value = 'completed'
        pushEvent(streamEvent, raw)
        closeSource()
        return

      case 'run.failed':
        clearQueuedTimer()
        error.value = streamEvent.payload?.error || ''
        status.value = 'failed'
        pushEvent(streamEvent, raw)
        closeSource()
        return

      default:
        pushEvent(streamEvent, raw)
    }
  }

  const connect = () => {
    if (!traceId) return

    closeSource()
    clearQueuedTimer()

    if (!events.value.length) {
      status.value = 'starting'
    }

    source = new EventSource(`/api/v1/ai/runs/${encodeURIComponent(traceId)}/stream`)
    source.onopen = () => {
      if (terminalStatuses.has(status.value)) return
      if (status.value === 'disconnected') {
        if (sawTurnActivity) {
          status.value = 'running'
        } else if (sawRunStarted) {
          status.value = 'starting'
        } else {
          status.value = 'queued'
        }
      }
    }
    source.onerror = () => {
      if (!terminalStatuses.has(status.value)) {
        status.value = 'disconnected'
      }
    }

    for (const type of replayableEventTypes) {
      source.addEventListener(type, (evt) => {
        try {
          const parsed = JSON.parse(evt.data)
          handleEvent(parsed, evt)
        } catch {
          if (!terminalStatuses.has(status.value)) {
            status.value = 'disconnected'
          }
        }
      })
    }
  }

  const disconnect = () => {
    clearQueuedTimer()
    closeSource()
    if (!terminalStatuses.has(status.value)) {
      status.value = 'idle'
    }
  }

  return {
    events,
    status,
    finalResult,
    executionId,
    error,
    connect,
    disconnect,
  }
}
