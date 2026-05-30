import { computed, ref } from 'vue'

const activePanel = ref(null)
const previousPanel = ref(null)

export function useAgentRightRail () {
  const isOpen = computed(() => activePanel.value !== null)

  const open = (panelName, { rememberPrevious = false } = {}) => {
    if (rememberPrevious && activePanel.value && activePanel.value !== panelName) {
      previousPanel.value = activePanel.value
    } else if (!rememberPrevious) {
      previousPanel.value = null
    }
    activePanel.value = panelName
  }

  const close = ({ restorePrevious = false } = {}) => {
    if (restorePrevious && previousPanel.value) {
      activePanel.value = previousPanel.value
      previousPanel.value = null
      return
    }
    activePanel.value = null
    previousPanel.value = null
  }

  const isActive = (panelName) => activePanel.value === panelName

  const reset = () => {
    activePanel.value = null
    previousPanel.value = null
  }

  return {
    activePanel,
    isOpen,
    open,
    close,
    isActive,
    reset,
  }
}
