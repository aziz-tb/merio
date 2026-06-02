import { computed, ref, watch } from 'vue'
import { useUserStore } from '@main/stores/user'

export const INBOX_VIEW_MODES = {
  COLUMNS: 'columns',
  TABLE: 'table'
}

const STORAGE_NAMESPACE = 'merio:inbox'
const viewMode = ref(INBOX_VIEW_MODES.COLUMNS)
let initialized = false

export function inboxStorageKey(suffix, userID = 'global') {
  return `${STORAGE_NAMESPACE}:${suffix}:${userID || 'global'}`
}

function normalizeViewMode(mode) {
  return mode === INBOX_VIEW_MODES.TABLE
    ? INBOX_VIEW_MODES.TABLE
    : INBOX_VIEW_MODES.COLUMNS
}

function readViewMode(key) {
  try {
    return normalizeViewMode(window.localStorage.getItem(key))
  } catch (error) {
    console.warn('[useInboxView] failed to read view mode:', error)
    return INBOX_VIEW_MODES.COLUMNS
  }
}

function writeViewMode(key, mode) {
  try {
    window.localStorage.setItem(key, normalizeViewMode(mode))
  } catch (error) {
    console.warn('[useInboxView] failed to save view mode:', error)
  }
}

export function useInboxView() {
  const userStore = useUserStore()
  const storageUserID = computed(() => userStore.userID || 'global')
  const viewModeKey = computed(() => inboxStorageKey('view-mode', storageUserID.value))

  if (!initialized) {
    initialized = true

    watch(
      viewModeKey,
      (key) => {
        viewMode.value = readViewMode(key)
      },
      { immediate: true }
    )

    watch(viewMode, (mode) => {
      writeViewMode(viewModeKey.value, mode)
    })
  }

  const isTableView = computed(() => viewMode.value === INBOX_VIEW_MODES.TABLE)

  const setViewMode = (mode) => {
    viewMode.value = normalizeViewMode(mode)
  }

  const toggleViewMode = () => {
    viewMode.value = isTableView.value
      ? INBOX_VIEW_MODES.COLUMNS
      : INBOX_VIEW_MODES.TABLE
  }

  const getInboxStorageKey = (suffix) => inboxStorageKey(suffix, storageUserID.value)

  return {
    viewMode,
    isTableView,
    setViewMode,
    toggleViewMode,
    getInboxStorageKey,
    storageUserID
  }
}
