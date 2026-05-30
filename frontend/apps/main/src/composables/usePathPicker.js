import { computed, ref } from 'vue'
import { StorageSerializers, useSessionStorage } from '@vueuse/core'
import { useAgentRightRail } from './useAgentRightRail'

const targetConditionContext = ref(null)
const lastIdentifier = useSessionStorage('libredesk:path-picker-identifier', '')
const lastSource = useSessionStorage('libredesk:path-picker-source', 'ticket')
const lastConversation = useSessionStorage('libredesk:path-picker-conversation', null, {
  serializer: StorageSerializers.object,
})

export function usePathPicker () {
  const rail = useAgentRightRail()
  const isOpen = computed(() => rail.isActive('path-picker'))

  const open = (context) => {
    targetConditionContext.value = context
    rail.open('path-picker', { rememberPrevious: true })
  }

  const close = () => {
    targetConditionContext.value = null
    if (rail.isActive('path-picker')) {
      rail.close({ restorePrevious: true })
    }
  }

  const reset = () => {
    targetConditionContext.value = null
    if (rail.isActive('path-picker')) {
      rail.close()
    }
  }

  const selectPath = (selection) => {
    const ctx = targetConditionContext.value
    if (ctx?.onSelect) ctx.onSelect(selection)
    close()
  }

  return {
    isOpen,
    targetConditionContext,
    lastIdentifier,
    lastSource,
    lastConversation,
    open,
    close,
    reset,
    selectPath,
  }
}
