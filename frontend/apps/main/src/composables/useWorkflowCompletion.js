import { onMounted, onUnmounted, unref } from 'vue'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { useEmitter } from '@/composables/useEmitter'

export function shouldHandleWorkflowCompletion(event, conversationUuid) {
  return Boolean(event?.conversation_uuid && conversationUuid && event.conversation_uuid === conversationUuid)
}

export function createWorkflowCompletionHandler(conversationUuidRef, callback) {
  return (event) => {
    if (!shouldHandleWorkflowCompletion(event, unref(conversationUuidRef))) return
    callback(event)
  }
}

export function useWorkflowCompletion(eventName, conversationUuidRef, callback) {
  const emitter = useEmitter()
  const emitterEvent = EMITTER_EVENTS[eventName] || eventName
  const handler = createWorkflowCompletionHandler(conversationUuidRef, callback)

  onMounted(() => emitter.on(emitterEvent, handler))
  onUnmounted(() => emitter.off(emitterEvent, handler))
}
