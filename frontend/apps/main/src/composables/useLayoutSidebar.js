// Shared open/closed state for the secondary sidebar.
// Persisted in localStorage so the choice survives reloads.
//
// Read by:
//   - Sidebar.vue (slides the aside off-canvas when closed)
//   - App.vue (collapses main content margin-start when closed)
//   - PageHeader.vue (toggle button)
import { useStorage } from '@vueuse/core'

const open = useStorage('mainSidebarOpen', true)

export function useLayoutSidebar() {
  return {
    open,
    toggle: () => {
      open.value = !open.value
    }
  }
}
