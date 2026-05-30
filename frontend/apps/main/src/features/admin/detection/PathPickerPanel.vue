<template>
  <aside class="w-full shrink-0 flex flex-col h-full overflow-hidden bg-background">
    <div class="flex items-center justify-between gap-3 border-b border-input px-4 py-3 shrink-0">
      <div>
        <h3 class="text-sm font-semibold">{{ t('admin.detection.pathPicker') }}</h3>
        <p class="text-xs text-muted-foreground">{{ sourceLabel(source) }}</p>
      </div>
      <Button type="button" variant="ghost" size="icon" @click="picker.close()">
        <X class="size-4" />
      </Button>
    </div>

    <div class="flex-1 overflow-y-auto">
      <div class="space-y-3 mb-2 px-4 py-3">
        <div v-if="source !== 'tags'" class="space-y-1">
          <Label class="text-xs">{{ t('admin.detection.conversationIdentifier') }}</Label>

          <div
            v-if="selectedConversation"
            class="flex min-w-0 items-center justify-between gap-2 rounded-md border border-input bg-muted/30 px-2.5 py-1.5"
          >
            <div class="min-w-0 text-xs">
              <p class="truncate font-medium">{{ conversationLabel(selectedConversation) }}</p>
              <p v-if="selectedConversation.status" class="truncate text-muted-foreground">
                {{ selectedConversation.status }}
              </p>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              class="size-6 shrink-0"
              :title="t('admin.detection.pathPicker.clearSelection')"
              :aria-label="t('admin.detection.pathPicker.clearSelection')"
              @click="clearConversation"
            >
              <X class="size-3.5" />
            </Button>
          </div>

          <div v-else class="space-y-2">
            <div class="relative">
              <Search class="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                v-model="searchQuery"
                class="h-9 pl-8"
                :placeholder="t('admin.detection.pathPicker.conversationSearchPlaceholder')"
                autocomplete="off"
                @focus="onInputFocus"
                @keydown.down.prevent="onArrowDown"
                @keydown.up.prevent="onArrowUp"
                @keydown.enter.prevent="onEnter"
                @keydown.esc="closeDropdown"
              />
            </div>

            <div
              v-if="dropdownOpen"
              ref="dropdownList"
              class="max-h-72 overflow-y-auto rounded-md border border-input bg-background py-1"
            >
              <div v-if="searching" class="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
                <Loader2 class="size-3.5 animate-spin" />
                {{ t('admin.detection.pathPicker.searching') }}
              </div>

              <div v-else-if="searchError" class="px-3 py-2 text-xs text-muted-foreground">
                {{ t('admin.detection.pathPicker.loadError') }}
              </div>

              <div
                v-else-if="searchQuery.trim().length >= SEARCH_MIN_CHARS && searchResults.length === 0"
                class="px-3 py-2 text-xs text-muted-foreground"
              >
                {{ t('admin.detection.pathPicker.noConversationsFound') }}
              </div>

              <template v-else>
                <div v-for="(result, index) in searchResults" :key="result.uuid" :data-index="index">
                  <button
                    type="button"
                    class="flex w-full items-start gap-2 px-3 py-2 text-left transition-colors"
                    :class="index === highlightedIndex ? 'bg-muted' : 'hover:bg-muted'"
                    @click="selectConversation(result)"
                    @mousemove="highlightedIndex = index"
                  >
                    <div class="min-w-0">
                      <div class="truncate text-sm">
                        <span class="font-mono font-medium">#{{ result.reference_number }}</span>
                        <span v-if="result.contact_name" class="ml-2 text-muted-foreground">
                          {{ result.contact_name }}
                        </span>
                      </div>
                      <div class="truncate text-xs text-muted-foreground">
                        {{ result.last_message_snippet || result.subject || result.contact_email || t('globals.messages.noSubject') }}
                      </div>
                      <div class="truncate text-xs text-muted-foreground">
                        <template v-if="result.contact_email">{{ result.contact_email }} · </template>
                        {{ result.status || '-' }}
                        <template v-if="result.created_at"> · {{ formatRelative(result.created_at) }}</template>
                      </div>
                    </div>
                  </button>
                </div>
              </template>
            </div>
          </div>

          <p v-if="loadError" class="text-xs text-destructive">{{ loadError }}</p>
        </div>

        <div class="space-y-1">
          <Label class="text-xs">{{ t('admin.detection.source.label') }}</Label>
          <Select v-model="source">
            <SelectTrigger>
              <SelectValue :placeholder="t('admin.detection.selectSource')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="item in availableSources" :key="item" :value="item">
                {{ sourceLabel(item) }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div v-if="source === 'tags'" class="p-4 space-y-3">
        <TagMultiSelect v-model="selectedTags" />
        <Button type="button" class="w-full" @click="selectTags">
          {{ t('admin.detection.useSelectedTags') }}
        </Button>
      </div>

      <div v-else-if="loading" class="px-4 py-8 text-center text-sm text-muted-foreground">
        <Loader2 class="mx-auto mb-2 size-4 animate-spin" />
        {{ t('globals.messages.loading') }}
      </div>

      <div v-else-if="!selectedConversation" class="px-4 py-8 text-center text-sm text-muted-foreground">
        {{ t('admin.detection.pathPicker.selectConversationFirst') }}
      </div>

      <div v-else-if="!variables" class="px-4 py-8 text-center text-sm text-muted-foreground">
        {{ t('admin.detection.pathPicker.loadHint') }}
      </div>

      <div
        v-else-if="source === 'customer' && !variables.customer"
        class="px-4 py-8 text-center text-sm text-muted-foreground"
      >
        {{ t('admin.detection.pathPicker.noCustomer') }}
      </div>

      <div
        v-else-if="source === 'contract' && !variables.contract"
        class="px-4 py-8 text-center text-sm text-muted-foreground"
      >
        {{ t('admin.detection.pathPicker.noContract') }}
      </div>

      <div v-else-if="treeData" class="p-2">
        <JsonTreeViewer
          :value="treeData"
          mode="picker"
          @select-path="onPathSelect"
        />
      </div>
    </div>
  </aside>
</template>

<script setup>
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDebounceFn } from '@vueuse/core'
import { Loader2, Search, X } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import api from '@/api'
import { usePathPicker } from '@/composables/usePathPicker'
import JsonTreeViewer from './JsonTreeViewer.vue'
import TagMultiSelect from './TagMultiSelect.vue'

const { t } = useI18n()
const picker = usePathPicker()

const context = computed(() => picker.targetConditionContext.value || {})
const availableSources = computed(() => context.value.availableSources || ['ticket', 'customer', 'contract', 'tags'])
const source = picker.lastSource
const conversationIdentifier = picker.lastIdentifier
const selectedConversation = picker.lastConversation

const loading = ref(false)
const loadError = ref('')
const variables = ref(null)
const selectedTags = ref([])
const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)
const searchError = ref(false)
const dropdownOpen = ref(false)
const highlightedIndex = ref(-1)
const dropdownList = ref(null)

const SEARCH_MIN_CHARS = 2
const SEARCH_DEBOUNCE_MS = 300
const SEARCH_MAX_RESULTS = 10
let searchAbort = null
let searchSeq = 0

const treeData = computed(() => {
  if (!variables.value || source.value === 'tags') return null
  return variables.value[source.value] || null
})

const sourceLabel = (value) => ({
  ticket: t('admin.detection.source.ticket'),
  customer: t('admin.detection.source.customer'),
  contract: t('admin.detection.source.contract'),
  tags: t('admin.detection.source.tags')
})[value] || value

const loadVariables = async () => {
  const identifier = conversationIdentifier.value.trim()
  if (!identifier) return
  try {
    loading.value = true
    loadError.value = ''
    const resp = await api.getConversationVariables(identifier)
    variables.value = resp.data?.data || null
  } catch (error) {
    loadError.value = error?.response?.data?.message || t('admin.detection.pathPicker.loadError')
    variables.value = null
  } finally {
    loading.value = false
  }
}

const cancelInFlightSearch = () => {
  if (searchAbort) {
    searchAbort.abort()
    searchAbort = null
  }
}

const closeDropdown = () => {
  dropdownOpen.value = false
  highlightedIndex.value = -1
}

const resetSearchToIdle = () => {
  cancelInFlightSearch()
  searchSeq += 1
  searchResults.value = []
  searchError.value = false
  searching.value = false
  closeDropdown()
}

const performSearch = async () => {
  const q = searchQuery.value.trim()
  if (q.length < SEARCH_MIN_CHARS) {
    resetSearchToIdle()
    return
  }

  cancelInFlightSearch()
  searchAbort = new AbortController()
  const controller = searchAbort
  const seq = (searchSeq += 1)
  searching.value = true
  searchError.value = false
  highlightedIndex.value = -1
  dropdownOpen.value = true

  try {
    const resp = await api.searchConversations({ query: q }, { signal: controller.signal })
    if (seq !== searchSeq) return
    const data = Array.isArray(resp.data?.data) ? resp.data.data : []
    searchResults.value = data.slice(0, SEARCH_MAX_RESULTS)
  } catch (error) {
    if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') return
    if (seq !== searchSeq) return
    searchResults.value = []
    searchError.value = true
  } finally {
    if (seq === searchSeq) searching.value = false
  }
}

const debouncedSearch = useDebounceFn(performSearch, SEARCH_DEBOUNCE_MS)

const onInputFocus = () => {
  if (
    searchQuery.value.trim().length >= SEARCH_MIN_CHARS &&
    (searchResults.value.length || searching.value || searchError.value)
  ) {
    dropdownOpen.value = true
  }
}

const scrollHighlightedIntoView = () => {
  nextTick(() => {
    const el = dropdownList.value?.querySelector(`[data-index="${highlightedIndex.value}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  })
}

const onArrowDown = () => {
  if (!dropdownOpen.value) {
    if (searchQuery.value.trim().length >= SEARCH_MIN_CHARS && searchResults.value.length) {
      dropdownOpen.value = true
    }
    return
  }
  if (!searchResults.value.length) return
  highlightedIndex.value = Math.min(highlightedIndex.value + 1, searchResults.value.length - 1)
  scrollHighlightedIntoView()
}

const onArrowUp = () => {
  if (!dropdownOpen.value || !searchResults.value.length) return
  highlightedIndex.value = Math.max(highlightedIndex.value - 1, 0)
  scrollHighlightedIntoView()
}

const onEnter = () => {
  if (!dropdownOpen.value) return
  const result = searchResults.value[highlightedIndex.value] || searchResults.value[0]
  if (result) selectConversation(result)
}

const selectConversation = async (result) => {
  const normalized = {
    uuid: result.uuid,
    reference_number: result.reference_number,
    subject: result.subject || '',
    status: result.status || '',
    created_at: result.created_at || '',
    contact_name: result.contact_name || '',
    contact_email: result.contact_email || '',
    last_message_snippet: result.last_message_snippet || '',
  }
  selectedConversation.value = normalized
  conversationIdentifier.value = normalized.uuid
  searchQuery.value = ''
  resetSearchToIdle()
  await loadVariables()
}

const clearConversation = () => {
  conversationIdentifier.value = ''
  selectedConversation.value = null
  variables.value = null
  loadError.value = ''
  searchQuery.value = ''
  resetSearchToIdle()
}

const conversationLabel = (conversation) => {
  if (!conversation) return ''
  const ref = conversation.reference_number ? `#${conversation.reference_number}` : conversation.uuid
  return conversation.subject ? `${ref} · ${conversation.subject}` : ref
}

const formatRelative = (iso) => {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diff = Date.now() - then
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

const onPathSelect = (path) => {
  picker.selectPath({ source: source.value, path })
}

const selectTags = () => {
  picker.selectPath({ source: 'tags', path: '', operator: 'in', value: selectedTags.value })
}

watch(
  availableSources,
  (items) => {
    if (!items.includes(source.value)) {
      source.value = items[0] || 'ticket'
    }
  },
  { immediate: true }
)

watch(searchQuery, (value) => {
  if ((value || '').trim().length < SEARCH_MIN_CHARS) {
    resetSearchToIdle()
    return
  }
  debouncedSearch()
})

watch(
  () => picker.isOpen.value,
  (open) => {
    if (open && conversationIdentifier.value && !variables.value && source.value !== 'tags') {
      loadVariables()
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  cancelInFlightSearch()
})
</script>
