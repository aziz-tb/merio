<!-- frontend/apps/main/src/features/conversation/list/ConversationTable.vue -->

<template>
  <div class="flex h-full min-h-0 flex-col bg-sidebar">
    <!-- Top toolbar: search + count pill (no filter/sort icons) -->
    <div class="flex shrink-0 items-center gap-3 border-b border-border p-4">
      <div class="relative max-w-sm flex-1">
        <Search class="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          v-model="searchQuery"
          :placeholder="t('conversation.table.searchPlaceholder')"
          class="h-8 border-border/60 bg-transparent pl-9 text-xs shadow-none focus-visible:bg-background"
        />
      </div>
      <DropdownMenu v-if="!route.params.viewID">
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            class="h-7 shrink-0 gap-1 rounded-full border border-border bg-background px-3 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <span>{{ tableTotal }}</span>
            <span>{{ listStatusLabel }}</span>
            <ChevronDown class="size-3 opacity-50" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            v-for="status in conversationStore.statusOptions"
            :key="status.value"
            @click="handleStatusChange(status)"
          >
            {{ statusDisplayLabel(status) }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <span
        v-else
        class="inline-flex h-7 shrink-0 items-center rounded-full border border-border bg-background px-3 text-xs text-muted-foreground"
      >
        {{ tableTotal }}
      </span>
    </div>

    <!-- Scrollable table -->
    <div ref="scrollContainer" class="min-h-0 flex-1 overflow-auto">
      <table class="w-full border-collapse text-sm" :style="{ minWidth: tableMinWidth }">
        <thead class="sticky top-0 z-20 bg-sidebar">
          <tr ref="headerRowEl" class="m-2">
            <th
              v-for="column in visibleColumns"
              :key="column.id"
              :class="[
                'group/header relative h-9 px-3 text-left text-[11px] font-medium uppercase tracking-wider text-muted-foreground/80',
                column.align === 'center' ? 'text-center' : 'text-left',
                isLastVisibleColumn(column.id) ? 'pr-10' : ''
              ]"
              :style="columnStyle(column)"
            >
              <div
                :class="[
                  'flex min-w-0 items-center gap-1.5',
                  column.align === 'center' ? 'justify-center' : 'justify-start'
                ]"
              >
                <GripVertical
                  class="column-drag-handle size-3 shrink-0 cursor-grab text-muted-foreground/60 opacity-0 transition-opacity group-hover/header:opacity-100 active:cursor-grabbing"
                  :aria-label="t('conversation.table.dragToReorder')"
                />
                <span class="truncate normal-case tracking-normal text-xs text-muted-foreground">{{ column.label }}</span>
                <ArrowUp
                  v-if="sortState?.id === column.id && !sortState.desc"
                  class="size-3 shrink-0 text-foreground"
                />
                <ArrowDown
                  v-else-if="sortState?.id === column.id && sortState.desc"
                  class="size-3 shrink-0 text-foreground"
                />

                <DropdownMenu>
                  <DropdownMenuTrigger
                    class="ml-auto inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground/60 opacity-0 transition-opacity hover:bg-muted hover:text-foreground data-[state=open]:opacity-100 group-hover/header:opacity-100"
                    @click.stop
                  >
                    <ChevronDown class="size-4" />
                    <span class="sr-only">{{ t('globals.terms.openMenu') }}</span>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" class="w-44">
                    <DropdownMenuItem @click.stop="setSort(column.id, false)">
                      <Check
                        :class="[
                          'mr-2 size-4',
                          isSorted(column.id, false) ? 'opacity-100' : 'opacity-0'
                        ]"
                      />
                      {{ t('conversation.table.sortAscending') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem @click.stop="setSort(column.id, true)">
                      <Check
                        :class="[
                          'mr-2 size-4',
                          isSorted(column.id, true) ? 'opacity-100' : 'opacity-0'
                        ]"
                      />
                      {{ t('conversation.table.sortDescending') }}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      :disabled="visibleColumns.length <= 1"
                      @click.stop="hideColumn(column.id)"
                    >
                      <EyeOff class="mr-2 size-4" />
                      {{ t('conversation.table.hideColumn') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
              <!-- Right-edge resize handle (visible on hover) -->
              <button
                type="button"
                class="absolute right-0 top-1.5 bottom-1.5 w-px cursor-col-resize bg-transparent transition-colors hover:w-0.5 hover:bg-border group-hover/header:bg-border/60"
                :aria-label="t('conversation.table.resizeColumn')"
                @pointerdown.stop.prevent="startResize(column.id, $event)"
              />
              <DropdownMenu v-if="isLastVisibleColumn(column.id)">
                <DropdownMenuTrigger
                  class="no-drag absolute right-2 top-1.5 inline-flex size-6 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground"
                  :title="t('conversation.table.columnPicker')"
                  @click.stop
                >
                  <Plus class="size-4" />
                  <span class="sr-only">{{ t('conversation.table.columnPicker') }}</span>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" class="w-56">
                  <DropdownMenuLabel>{{ t('conversation.table.columnPicker') }}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuCheckboxItem
                    v-for="column in columnsByDefinition"
                    :key="column.id"
                    :checked="isColumnVisible(column.id)"
                    :disabled="isColumnVisible(column.id) && visibleColumns.length <= 1"
                    @update:checked="(value) => toggleColumn(column.id, value)"
                  >
                    {{ column.label }}
                  </DropdownMenuCheckboxItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </th>
          </tr>
        </thead>

        <tbody>
          <!-- Skeleton -->
          <template v-if="showSkeleton">
            <tr v-for="row in 8" :key="row" class="border-b border-border/40">
              <td
                v-for="column in visibleColumns"
                :key="column.id"
                class="h-14 px-3"
                :style="columnStyle(column)"
              >
                <Skeleton class="h-3.5 w-full max-w-[12rem]" />
              </td>
            </tr>
          </template>

          <!-- Empty state -->
          <tr v-else-if="sortedRows.length === 0" class="hover:bg-transparent">
            <td :colspan="tableColumnCount" class="h-64 px-4">
              <div class="flex flex-col items-center justify-center gap-1 text-center">
                <p class="text-sm font-medium text-foreground">
                  {{ t('conversation.table.noConversations') }}
                </p>
                <p class="text-xs text-muted-foreground">
                  {{ t('conversation.table.emptyHint') }}
                </p>
              </div>
            </td>
          </tr>

          <!-- Rows -->
          <template v-else>
            <tr
              v-for="conversation in sortedRows"
              :key="conversation.uuid"
              class="group cursor-pointer border-b border-border/40 transition-colors hover:bg-muted/40"
              @click="openConversation(conversation)"
            >
              <td
                v-for="column in visibleColumns"
                :key="column.id"
                class="h-20 px-3 align-middle text-sm"
                :style="columnStyle(column)"
              >
                <ConversationTableCell
                  :column-id="column.id"
                  :conversation="conversation"
                  :now="now"
                />
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <!-- Load more -->
    <div
      v-if="conversationStore.conversations.hasMore"
      class="flex h-11 shrink-0 items-center justify-center border-t border-border"
    >
      <Button
        variant="ghost"
        size="sm"
        class="h-7 text-xs"
        :disabled="isLoading"
        @click="conversationStore.fetchNextConversations"
      >
        <Loader v-if="isLoading" class="mr-2 size-4 animate-spin" />
        {{ isLoading ? t('globals.terms.loading') : t('globals.terms.loadMore') }}
      </Button>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDraggable } from 'vue-draggable-plus'
import { useI18n } from 'vue-i18n'
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronDown,
  EyeOff,
  GripVertical,
  Loader,
  Plus,
  Search
} from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Skeleton } from '@shared-ui/components/ui/skeleton'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { useConversationStore } from '@main/stores/conversation'
import { useTeamStore } from '@main/stores/team'
import { useUsersStore } from '@main/stores/users'
import { useInboxView } from '@main/composables/useInboxView'
import { getStatusDisplayLabel } from '@main/utils/statusVisuals'
import ConversationTableCell from '@/features/conversation/list/ConversationTableCell.vue'

const MIN_COLUMN_WIDTH = 72
const MAX_COLUMN_WIDTH = 560

// Column definitions. Each column id matches a branch in ConversationTableCell.
// `subject` column shows the subject line + last message preview stacked.
// `user` column shows the contact avatar + name + email.
const COLUMN_DEFS = [
  {
    id: 'subject',
    labelKey: 'conversation.table.columns.subject',
    width: 360,
    sortType: 'text',
    sortValue: (row) => row.subject || '',
    searchValue: (row) =>
      [row.subject, cleanText(row.last_interaction || row.last_message)].filter(Boolean).join(' ')
  },
  {
    id: 'user',
    labelKey: 'conversation.table.columns.user',
    width: 240,
    sortType: 'text',
    sortValue: (row) => contactName(row),
    searchValue: (row) => [contactName(row), row.contact?.email].filter(Boolean).join(' ')
  },
  {
    id: 'priority',
    labelKey: 'conversation.table.columns.priority',
    width: 128,
    align: 'center',
    sortType: 'number',
    sortValue: (row) => priorityRank(row.priority),
    searchValue: (row) => row.priority || ''
  },
  {
    id: 'waiting_since',
    labelKey: 'conversation.table.columns.waitingSince',
    width: 116,
    sortType: 'date',
    sortValue: (row) => row.waiting_since,
    searchValue: (row) => row.waiting_since || ''
  },
  {
    id: 'last_activity',
    labelKey: 'conversation.table.columns.lastActivity',
    width: 116,
    sortType: 'date',
    sortValue: (row) => row.last_interaction_at || row.last_message_at,
    searchValue: (row) => row.last_interaction_at || row.last_message_at || ''
  },
  {
    id: 'next_sla',
    labelKey: 'conversation.table.columns.nextSla',
    width: 104,
    sortType: 'date',
    sortValue: (row) => row.next_sla_deadline_at,
    searchValue: (row) => row.next_sla_deadline_at || ''
  },
  {
    id: 'status',
    labelKey: 'conversation.table.columns.status',
    width: 124,
    sortType: 'text',
    sortValue: (row) => row.status || '',
    searchValue: (row) => row.status || ''
  },
  {
    id: 'team',
    labelKey: 'conversation.table.columns.team',
    width: 140,
    sortType: 'text',
    sortValue: (row) => assignedTeamName(row),
    searchValue: (row) => assignedTeamName(row)
  },
  {
    id: 'assignee',
    labelKey: 'conversation.table.columns.assignee',
    width: 170,
    sortType: 'text',
    sortValue: (row) => assignedUserName(row),
    searchValue: (row) => assignedUserName(row)
  },
  {
    id: 'tags',
    labelKey: 'conversation.table.columns.tags',
    width: 180,
    sortType: 'text',
    sortValue: (row) => tagText(row),
    searchValue: (row) => tagText(row)
  },
  {
    id: 'date_started',
    labelKey: 'conversation.table.columns.dateStarted',
    width: 116,
    sortType: 'date',
    sortValue: (row) => row.created_at,
    searchValue: (row) => row.created_at || ''
  }
]

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const conversationStore = useConversationStore()
const teamStore = useTeamStore()
const usersStore = useUsersStore()
const { getInboxStorageKey, storageUserID } = useInboxView()

const searchQuery = ref('')
const now = ref(new Date())
const columnOrder = ref(COLUMN_DEFS.map((column) => column.id))
const columnVisibility = ref({})
const columnWidths = ref({})
const sortState = ref(null)
const headerRowEl = ref(null)
const draggableColumnIds = ref([])
let timer = null
let preferencesReady = false

const columnsByDefinition = computed(() =>
  COLUMN_DEFS.map((column) => ({
    ...column,
    label: t(column.labelKey),
    align: column.align || 'left'
  }))
)

const columnMap = computed(() =>
  Object.fromEntries(columnsByDefinition.value.map((column) => [column.id, column]))
)

const orderedColumns = computed(() =>
  columnOrder.value
    .map((id) => columnMap.value[id])
    .filter(Boolean)
)

const visibleColumns = computed(() =>
  orderedColumns.value.filter((column) => isColumnVisible(column.id))
)

const tableColumnCount = computed(() => visibleColumns.value.length)
const isLoading = computed(() => conversationStore.conversations.loading)
const showSkeleton = computed(() => isLoading.value && conversationStore.conversationsList.length === 0)

const tableTotal = computed(() => conversationStore.conversations.total || sortedRows.value.length)
const statusDisplayLabel = (status) => getStatusDisplayLabel(status, t)
const listStatusLabel = computed(() => statusDisplayLabel(conversationStore.getListStatus))

const filteredRows = computed(() => {
  const rows = conversationStore.conversationsList || []
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return rows

  return rows.filter((row) => {
    const haystack = columnsByDefinition.value
      .map((column) => column.searchValue?.(row) ?? '')
      .join(' ')
      .toLowerCase()
    return haystack.includes(query)
  })
})

const sortedRows = computed(() => {
  if (!sortState.value?.id) return filteredRows.value
  const column = columnMap.value[sortState.value.id]
  if (!column) return filteredRows.value

  return [...filteredRows.value].sort((a, b) => {
    const result = compareValues(column.sortValue?.(a), column.sortValue?.(b), column.sortType)
    return sortState.value.desc ? -result : result
  })
})

const tableMinWidth = computed(() => {
  const width = visibleColumns.value.reduce((sum, column) => sum + columnWidth(column.id), 0)
  return `${Math.max(width, 720)}px`
})

watch(
  visibleColumns,
  (columns) => {
    draggableColumnIds.value = columns.map((column) => column.id)
  },
  { immediate: true }
)

watch(storageUserID, loadPreferences, { immediate: true })

watch(
  columnOrder,
  (value) => savePreference('table:column-order', value),
  { deep: true }
)
watch(
  columnVisibility,
  (value) => savePreference('table:column-visibility', value),
  { deep: true }
)
watch(
  columnWidths,
  (value) => savePreference('table:column-widths', value),
  { deep: true }
)
watch(
  sortState,
  (value) => savePreference('table:sort', value),
  { deep: true }
)

useDraggable(headerRowEl, draggableColumnIds, {
  animation: 150,
  handle: '.column-drag-handle',
  filter: '.no-drag',
  preventOnFilter: true,
  onMove: (event) => !event.related?.classList?.contains('no-drag'),
  onUpdate: () => {
    const nextVisible = [...draggableColumnIds.value]
    const hidden = columnOrder.value.filter((id) => !nextVisible.includes(id))
    columnOrder.value = [...nextVisible, ...hidden]
  }
})

onMounted(() => {
  timer = window.setInterval(() => {
    now.value = new Date()
  }, 60000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})

function isColumnVisible(id) {
  return columnVisibility.value[id] !== false
}

function toggleColumn(id, value) {
  if (value === false && visibleColumns.value.length <= 1 && isColumnVisible(id)) return
  columnVisibility.value = {
    ...columnVisibility.value,
    [id]: value !== false
  }
}

function hideColumn(id) {
  toggleColumn(id, false)
}

function isLastVisibleColumn(id) {
  const columns = visibleColumns.value
  return columns[columns.length - 1]?.id === id
}

function handleStatusChange(status) {
  conversationStore.setListStatus(status.label)
}

function setSort(id, desc) {
  sortState.value = { id, desc }
}

function isSorted(id, desc) {
  return sortState.value?.id === id && sortState.value.desc === desc
}

function columnWidth(id) {
  const fallback = columnMap.value[id]?.width || 160
  return clampWidth(columnWidths.value[id] || fallback)
}

function columnStyle(column) {
  const width = `${columnWidth(column.id)}px`
  return {
    width,
    minWidth: width,
    maxWidth: width
  }
}

function startResize(id, event) {
  const startX = event.clientX
  const startWidth = columnWidth(id)
  const previousCursor = document.body.style.cursor
  document.body.style.cursor = 'col-resize'

  const onMove = (moveEvent) => {
    columnWidths.value = {
      ...columnWidths.value,
      [id]: clampWidth(startWidth + moveEvent.clientX - startX)
    }
  }

  const onUp = () => {
    document.body.style.cursor = previousCursor
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }

  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

async function openConversation(conversation) {
  const baseRoute = route.name.includes('team')
    ? 'team-inbox-conversation'
    : route.name.includes('view')
      ? 'view-inbox-conversation'
      : 'inbox-conversation'

  await router.push({
    name: baseRoute,
    params: {
      uuid: conversation.uuid,
      ...(baseRoute === 'team-inbox-conversation' && { teamID: route.params.teamID }),
      ...(baseRoute === 'view-inbox-conversation' && { viewID: route.params.viewID })
    },
    query: conversation.mentioned_message_uuid
      ? { scrollTo: conversation.mentioned_message_uuid }
      : {}
  })
}

function loadPreferences() {
  preferencesReady = false
  columnOrder.value = normalizeOrder(readPreference('table:column-order'))
  columnVisibility.value = normalizeVisibility(readPreference('table:column-visibility'))
  columnWidths.value = normalizeWidths(readPreference('table:column-widths'))
  sortState.value = normalizeSort(readPreference('table:sort'))
  preferencesReady = true
}

function readPreference(suffix) {
  try {
    const raw = window.localStorage.getItem(getInboxStorageKey(suffix))
    return raw ? JSON.parse(raw) : null
  } catch (error) {
    console.warn(`[ConversationTable] failed to read ${suffix}:`, error)
    return null
  }
}

function savePreference(suffix, value) {
  if (!preferencesReady) return
  try {
    window.localStorage.setItem(getInboxStorageKey(suffix), JSON.stringify(value))
  } catch (error) {
    console.warn(`[ConversationTable] failed to save ${suffix}:`, error)
  }
}

function normalizeOrder(value) {
  const ids = COLUMN_DEFS.map((column) => column.id)
  const incoming = Array.isArray(value) ? value.filter((id) => ids.includes(id)) : []
  return [...incoming, ...ids.filter((id) => !incoming.includes(id))]
}

function normalizeVisibility(value) {
  const next = {}
  for (const column of COLUMN_DEFS) {
    next[column.id] = value?.[column.id] !== false
  }
  return next
}

function normalizeWidths(value) {
  const next = {}
  for (const column of COLUMN_DEFS) {
    next[column.id] = clampWidth(Number(value?.[column.id] || column.width))
  }
  return next
}

function normalizeSort(value) {
  const ids = new Set(COLUMN_DEFS.map((column) => column.id))
  if (!value || !ids.has(value.id)) return null
  return { id: value.id, desc: value.desc === true }
}

function compareValues(a, b, type = 'text') {
  if (!a && !b) return 0
  if (!a) return 1
  if (!b) return -1

  if (type === 'date') {
    return new Date(a).getTime() - new Date(b).getTime()
  }
  if (type === 'number') {
    return Number(a) - Number(b)
  }
  return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: 'base' })
}

function clampWidth(width) {
  return Math.min(MAX_COLUMN_WIDTH, Math.max(MIN_COLUMN_WIDTH, Number(width) || MIN_COLUMN_WIDTH))
}

function contactName(row = {}) {
  const contact = row?.contact || row || {}
  return [contact.first_name, contact.last_name].filter(Boolean).join(' ').trim()
}

function findByID(items, id) {
  if (id == null || id === '') return null
  return items.find((item) => String(item.id) === String(id)) || null
}

function assignedTeamName(row = {}) {
  return findByID(teamStore.teams, row.assigned_team_id)?.name || ''
}

function assignedUserName(row = {}) {
  return contactName(findByID(usersStore.users, row.assigned_user_id) || {})
}

function cleanText(value) {
  return String(value || '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()
}

function priorityRank(priority) {
  const ranks = { urgent: 4, high: 3, medium: 2, low: 1 }
  return ranks[String(priority || '').toLowerCase()] || 0
}

function tagText(row) {
  const tags = Array.isArray(row.tags) ? row.tags : []
  return tags
    .map((tag) => (typeof tag === 'string' ? tag : tag?.name))
    .filter(Boolean)
    .join(' ')
}
</script>
