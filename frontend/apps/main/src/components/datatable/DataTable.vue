<!-- frontend/apps/main/src/components/datatable/DataTable.vue

  Visual chrome matches ConversationTable: sticky white header, hover-revealed
  drag/menu/resize affordances per header, trailing sticky + column picker,
  subtle row borders and hover.

  Toolbar (top):       search + #toolbar + #toolbar-end + count pill
  Selection bar:       inline next to the count pill, only visible when rows
                       are selected (#selected-actions + cancel)
  Table region:        sticky header, sortable / drag-reorder / resize / hide
                       per column, trailing `+` column picker, virtualized
                       body when not in row-drag mode

  Opt-in via props:
    - selectable + v-model:selected     row checkboxes + select-all
    - draggable + @reorder              row drag via handle
    - columnVisibility                  enables the trailing + column picker
    - columnDraggable                   drag column headers to reorder
    - columnResizable                   right-edge resize handle per header
    - storageKey                        persists column visibility / order /
                                        widths / sort under
                                        `datatable:<storageKey>:<suffix>`
    - pagination='page'|'cursor'        bottom pagination
    - expandable + #expanded-row        per-row expansion

  Slots:
    - #toolbar / #toolbar-end           top toolbar controls
    - #selected-actions={ selected, clear }
                                        shown inline with count when selected
    - #expanded-row={ row }             content under an expanded row
    - #empty                            custom empty state
-->
<template>
  <LoadingOverlay :loading="loading" reserve-height>
    <div class="flex h-full min-h-0 w-full flex-col bg-background">
      <!-- Toolbar: always-visible top row -->
      <div class="flex shrink-0 items-center gap-3 p-4">
        <div v-if="searchable" class="relative max-w-sm flex-1">
          <Search class="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            v-model="globalFilter"
            :placeholder="searchPlaceholder || t('globals.terms.search')"
            class="h-8 border-border/60 bg-transparent pl-9 text-xs shadow-none focus-visible:bg-background"
          />
        </div>
        <div class="flex items-center gap-2">
          <slot name="toolbar" />
        </div>
        <div class="ml-auto flex items-center gap-2">
          <span class="inline-flex h-7 shrink-0 items-center rounded-full bg-background px-3 text-xs text-muted-foreground">
            <template v-if="selectedCount === 0">
              {{ t('datatable.itemCount', rows.length, { n: rows.length }) }}
            </template>
            <template v-else>
              {{ t('datatable.itemSelected', selectedCount, { n: selectedCount }) }}
            </template>
          </span>
          <slot name="toolbar-end" />
        </div>
      </div>

      <!-- Selection action bar: only when rows are selected -->
      <div
        v-if="selectedCount > 0"
        class="flex shrink-0 items-center gap-2 px-4 py-2"
      >
        <slot
          name="selected-actions"
          :selected="selectedRowData"
          :clear="clearSelection"
        />
        <Button
          variant="ghost"
          size="sm"
          class="h-7 text-xs ml-auto"
          @click="clearSelection"
        >
          {{ t('globals.messages.cancel') }}
        </Button>
      </div>
      <!-- Scrollable table -->
      <div
        ref="scrollContainer"
        class="min-h-0 flex-1 overflow-auto"
      >
        <table class="w-full border-collapse text-sm" :style="{ minWidth: tableMinWidth }">
          <!-- Header -->
          <TableHeader class="sticky top-0 z-20 bg-background">
            <TableRow
              ref="headerRowComponent"
              class="border-b border-border hover:bg-transparent"
            >
              <!-- Gutter cell (selection + row drag) -->
              <TableHead v-if="hasGutter" class="no-drag w-10 px-2">
                <div class="flex items-center justify-center">
                  <Checkbox
                    v-if="selectable"
                    :checked="selectAllState"
                    @update:checked="onSelectAll"
                    :aria-label="t('datatable.selectAll')"
                    :class="[
                      'transition-opacity',
                      selectedCount === 0 ? 'opacity-0 hover:opacity-100' : 'opacity-100'
                    ]"
                  />
                </div>
              </TableHead>

              <!-- Data column headers -->
              <th
                v-for="header in headerCells"
                :key="header.id"
                :class="[
                  'group/header relative h-9 px-3 text-[11px] font-medium text-muted-foreground/80',
                  alignClass(header.column.columnDef.meta?.align),
                  header.column.columnDef.meta?.isActions && 'no-drag'
                ]"
                :style="headerStyle(header)"
              >
                <div
                  :class="[
                    'flex min-w-0 items-center gap-1.5',
                    justifyClass(header.column.columnDef.meta?.align)
                  ]"
                >
                  <GripVertical
                    v-if="columnDraggable && !header.column.columnDef.meta?.isActions"
                    class="column-drag-handle size-3 shrink-0 cursor-grab text-muted-foreground/60 opacity-0 transition-opacity group-hover/header:opacity-100 active:cursor-grabbing"
                    :aria-label="t('datatable.dragToReorder')"
                  />
                  <span
                    :class="[
                      'truncate normal-case tracking-normal text-xs text-muted-foreground',
                      header.column.getCanSort() ? 'cursor-pointer select-none' : ''
                    ]"
                    @click="header.column.getCanSort() && header.column.getToggleSortingHandler()?.($event)"
                  >
                    <FlexRender
                      v-if="!header.isPlaceholder"
                      :render="header.column.columnDef.header"
                      :props="header.getContext()"
                    />
                  </span>
                  <ArrowUp
                    v-if="header.column.getIsSorted() === 'asc'"
                    class="size-3 shrink-0 text-foreground"
                  />
                  <ArrowDown
                    v-else-if="header.column.getIsSorted() === 'desc'"
                    class="size-3 shrink-0 text-foreground"
                  />

                  <DropdownMenu v-if="(header.column.getCanSort() || header.column.getCanHide()) && headerMenu">
                    <DropdownMenuTrigger
                      class="ml-auto inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground/60 opacity-0 transition-opacity hover:bg-muted hover:text-foreground data-[state=open]:opacity-100 group-hover/header:opacity-100"
                      @click.stop
                    >
                      <ChevronDown class="size-4" />
                      <span class="sr-only">{{ t('globals.terms.openMenu') }}</span>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" class="w-44">
                      <template v-if="header.column.getCanSort()">
                        <DropdownMenuItem @click.stop="header.column.toggleSorting(false)">
                          <Check
                            :class="[
                              'mr-2 size-4',
                              header.column.getIsSorted() === 'asc' ? 'opacity-100' : 'opacity-0'
                            ]"
                          />
                          {{ t('datatable.sortAscending') }}
                        </DropdownMenuItem>
                        <DropdownMenuItem @click.stop="header.column.toggleSorting(true)">
                          <Check
                            :class="[
                              'mr-2 size-4',
                              header.column.getIsSorted() === 'desc' ? 'opacity-100' : 'opacity-0'
                            ]"
                          />
                          {{ t('datatable.sortDescending') }}
                        </DropdownMenuItem>
                      </template>
                      <DropdownMenuSeparator
                        v-if="header.column.getCanSort() && header.column.getCanHide()"
                      />
                      <DropdownMenuItem
                        v-if="header.column.getCanHide()"
                        :disabled="visibleColumnCount <= 1"
                        @click.stop="header.column.toggleVisibility(false)"
                      >
                        <EyeOff class="mr-2 size-4" />
                        {{ t('datatable.hideColumn') }}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                <!-- Right-edge resize handle (visible on hover) -->
                <button
                  v-if="columnResizable && header.column.getCanResize()"
                  type="button"
                  class="absolute right-0 top-1.5 bottom-1.5 w-px cursor-col-resize bg-transparent transition-colors hover:w-0.5 hover:bg-border group-hover/header:bg-border/60"
                  :aria-label="t('datatable.resizeColumn')"
                  @mousedown.stop.prevent="header.getResizeHandler()?.($event)"
                  @touchstart.stop.prevent="header.getResizeHandler()?.($event)"
                />
              </th>

              <!-- Trailing + column picker (sticky right) -->
              <th
                v-if="columnVisibility"
                class="no-drag sticky right-0 z-10 h-9 w-10 bg-background px-1 text-center"
              >
                <DropdownMenu>
                  <DropdownMenuTrigger
                    class="inline-flex size-6 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground"
                    :title="t('datatable.columns')"
                  >
                    <Plus class="size-4" />
                    <span class="sr-only">{{ t('datatable.columns') }}</span>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-56">
                    <DropdownMenuLabel>{{ t('datatable.columns') }}</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuCheckboxItem
                      v-for="col in hideableColumns"
                      :key="col.id"
                      :checked="col.getIsVisible()"
                      :disabled="col.getIsVisible() && visibleColumnCount <= 1"
                      @update:checked="(v) => col.toggleVisibility(!!v)"
                    >
                      {{ columnLabel(col) }}
                    </DropdownMenuCheckboxItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </th>
            </TableRow>
          </TableHeader>

          <!--
            Body — single template. The wrapper element is
            VueDraggable in row-drag mode, TableBody otherwise.
            Padding rows for virtualization render only in the
            non-draggable branch so SortableJS's DOM↔model index
            mapping stays 1:1.
          -->
          <component
            v-if="rows.length"
            :is="draggable ? VueDraggable : TableBody"
            v-bind="bodyBindings"
          >
            <tr
              v-if="!draggable && paddingTop > 0"
              :style="{ height: `${paddingTop}px` }"
            />
            <template v-for="row in renderedRows" :key="row.id">
              <TableRow
                :data-state="row.getIsSelected() ? 'selected' : undefined"
                :class="rowClass(row)"
                @click="onRowClick(row)"
              >
                <!-- Gutter cell -->
                <TableCell v-if="hasGutter" class="w-10 px-2">
                  <div class="flex items-center justify-center gap-1">
                    <EllipsisVertical
                      v-if="draggable"
                      class="datatable-drag-handle size-4 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 cursor-grab active:cursor-grabbing shrink-0"
                      :title="t('datatable.dragToReorder')"
                    />
                    <Checkbox
                      v-if="selectable"
                      :checked="row.getIsSelected()"
                      @update:checked="(v) => row.toggleSelected(!!v)"
                      @click.stop
                      :class="[
                        'transition-opacity shrink-0',
                        row.getIsSelected() ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
                      ]"
                    />
                  </div>
                </TableCell>

                <!-- Data cells -->
                <TableCell
                  v-for="cell in row.getVisibleCells()"
                  :key="cell.id"
                  :class="[
                    'h-14 text-sm',
                    alignClass(cell.column.columnDef.meta?.align),
                    cell.column.columnDef.meta?.font === 'mono' && 'font-mono'
                  ]"
                  :style="cellStyle(cell)"
                >
                  <FlexRender
                    :render="cell.column.columnDef.cell"
                    :props="cell.getContext()"
                  />
                </TableCell>

                <!-- Trailing column placeholder -->
                <TableCell
                  v-if="columnVisibility"
                  class="h-14 w-10 sticky right-0 bg-background transition-colors group-hover:bg-muted/40"
                />
              </TableRow>

              <!-- Expansion row -->
              <TableRow
                v-if="expandable && expandedSet.has(row.id)"
                class="bg-muted/20 hover:bg-muted/20"
              >
                <TableCell :colspan="totalColumnCount" class="px-4 py-3">
                  <slot name="expanded-row" :row="row.original" />
                </TableCell>
              </TableRow>
            </template>
            <tr
              v-if="!draggable && paddingBottom > 0"
              :style="{ height: `${paddingBottom}px` }"
            />
          </component>

          <!-- Empty state -->
          <TableBody v-else>
            <TableRow class="hover:bg-transparent">
              <TableCell :colspan="totalColumnCount" class="h-64 px-4">
                <slot name="empty">
                  <div class="flex flex-col items-center justify-center gap-2 text-center">
                    <component :is="emptyIcon" class="h-8 w-8 text-muted-foreground/50" />
                    <p class="text-sm font-medium text-foreground">
                      {{ emptyTextResolved }}
                    </p>
                  </div>
                </slot>
              </TableCell>
            </TableRow>
          </TableBody>
        </table>
      </div>

      <!-- Pagination -->
      <PaginationBar
        v-if="pagination === 'page'"
        :page="page"
        :per-page="perPage"
        :total-pages="totalPages"
        :per-page-options="perPageOptions"
        class="shrink-0 border-t border-border"
        @update:page="(v) => emit('update:page', v)"
        @update:per-page="(v) => emit('update:perPage', v)"
      />
      <div
        v-else-if="pagination === 'cursor' && hasMore"
        class="flex h-11 shrink-0 items-center justify-center border-t border-border"
      >
        <Button
          variant="ghost"
          size="sm"
          class="h-7 text-xs"
          :disabled="loading"
          @click="emit('load-more')"
        >
          {{ loading ? t('globals.terms.loading') : t('globals.terms.loadMore') }}
        </Button>
      </div>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import {
  FlexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useVueTable
} from '@tanstack/vue-table'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { VueDraggable, useDraggable } from 'vue-draggable-plus'
import { useI18n } from 'vue-i18n'
import { ArrowDown, ArrowUp, Check, ChevronDown, EllipsisVertical, EyeSlash as EyeOff, Ghost, GripVertical, Plus, MagnifyingGlass as Search } from '@qall/icons'
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@shared-ui/components/ui/table'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PaginationBar from '@main/components/pagination/PaginationBar.vue'

const { t } = useI18n()

const props = defineProps({
  data: { type: Array, default: () => [] },
  columns: { type: Array, default: () => [] },
  rowId: { type: [String, Function], default: 'id' },

  loading: { type: Boolean, default: false },
  emptyText: { type: String, default: '' },
  emptyIcon: { type: Object, default: () => Ghost },

  searchable: { type: Boolean, default: false },
  searchPlaceholder: { type: String, default: '' },
  density: {
    type: String,
    default: 'comfortable',
    validator: (v) => ['comfortable', 'compact'].includes(v)
  },
  estimatedRowHeight: { type: Number, default: 56 },

  selectable: { type: Boolean, default: false },
  selected: { type: Array, default: () => [] },
  draggable: { type: Boolean, default: false },
  columnVisibility: { type: Boolean, default: false },
  columnDraggable: { type: Boolean, default: false },
  columnResizable: { type: Boolean, default: false },
  storageKey: { type: String, default: '' },
  headerMenu: { type: Boolean, default: false },

  pagination: {
    type: String,
    default: 'none',
    validator: (v) => ['none', 'page', 'cursor'].includes(v)
  },
  page: { type: Number, default: 1 },
  perPage: { type: Number, default: 25 },
  totalPages: { type: Number, default: 1 },
  perPageOptions: { type: Array, default: () => [15, 30, 50, 100] },
  hasMore: { type: Boolean, default: false },

  expandable: { type: Boolean, default: false }
})

const emit = defineEmits([
  'update:selected',
  'reorder',
  'update:page',
  'update:perPage',
  'load-more',
  'row-click'
])

// ──────────────────────────────────────────────────────────────────
// Row identity
// ──────────────────────────────────────────────────────────────────
const resolveRowId = (row) =>
  typeof props.rowId === 'function'
    ? String(props.rowId(row))
    : String(row?.[props.rowId] ?? '')

// ──────────────────────────────────────────────────────────────────
// Data — internalData always mirrors props.data, drag mode reorders
// it locally. TanStack reads from internalData directly.
// ──────────────────────────────────────────────────────────────────
const internalData = ref(props.data ? [...props.data] : [])
watch(
  () => props.data,
  (next) => {
    internalData.value = next ? [...next] : []
  }
)

// ──────────────────────────────────────────────────────────────────
// Selection — local ref + watch sync. A computed bridge introduces a
// 1-tick lag that breaks the select-all cascade; keep the watch.
// ──────────────────────────────────────────────────────────────────
const rowSelectionState = ref({})

watch(
  () => props.selected,
  (selected) => {
    const incoming = {}
    for (const id of selected || []) incoming[String(id)] = true
    const incomingKey = Object.keys(incoming).sort().join('|')
    const currentKey = Object.keys(rowSelectionState.value)
      .filter((k) => rowSelectionState.value[k])
      .sort()
      .join('|')
    if (incomingKey !== currentKey) rowSelectionState.value = incoming
  },
  { immediate: true }
)

const selectedCount = computed(
  () => Object.values(rowSelectionState.value).filter(Boolean).length
)
const selectedRowData = computed(() => {
  const ids = new Set(
    Object.keys(rowSelectionState.value).filter(
      (k) => rowSelectionState.value[k]
    )
  )
  return internalData.value.filter((r) => ids.has(resolveRowId(r)))
})
function clearSelection() {
  rowSelectionState.value = {}
  emit('update:selected', [])
}

// ──────────────────────────────────────────────────────────────────
// TanStack table
// ──────────────────────────────────────────────────────────────────
const sorting = ref([])
const globalFilter = ref('')
const columnVisibilityState = ref({})
const columnOrder = ref([])
const columnSizing = ref({})

const table = useVueTable({
  get data() { return internalData.value },
  get columns() { return props.columns },
  state: {
    get sorting() { return sorting.value },
    get globalFilter() { return globalFilter.value },
    get rowSelection() { return rowSelectionState.value },
    get columnVisibility() { return columnVisibilityState.value },
    get columnOrder() { return columnOrder.value },
    get columnSizing() { return columnSizing.value }
  },
  enableSortingRemoval: false,
  columnResizeMode: 'onChange',
  getRowId: resolveRowId,
  onSortingChange: (u) =>
    (sorting.value = typeof u === 'function' ? u(sorting.value) : u),
  onGlobalFilterChange: (u) =>
    (globalFilter.value = typeof u === 'function' ? u(globalFilter.value) : u),
  onRowSelectionChange: (u) => {
    const next = typeof u === 'function' ? u(rowSelectionState.value) : u
    rowSelectionState.value = next
    emit('update:selected', Object.keys(next).filter((k) => next[k]))
  },
  onColumnVisibilityChange: (u) =>
    (columnVisibilityState.value =
      typeof u === 'function' ? u(columnVisibilityState.value) : u),
  onColumnOrderChange: (u) =>
    (columnOrder.value =
      typeof u === 'function' ? u(columnOrder.value) : u),
  onColumnSizingChange: (u) =>
    (columnSizing.value =
      typeof u === 'function' ? u(columnSizing.value) : u),
  getCoreRowModel: getCoreRowModel(),
  getSortedRowModel: getSortedRowModel(),
  getFilteredRowModel: getFilteredRowModel()
})

const rows = computed(() => table.getRowModel().rows)
const headerCells = computed(
  () => table.getHeaderGroups()[0]?.headers ?? []
)
const visibleColumnCount = computed(() => table.getVisibleLeafColumns().length)

// ──────────────────────────────────────────────────────────────────
// Gutter + total column count for colspans
// ──────────────────────────────────────────────────────────────────
const hasGutter = computed(() => props.selectable || props.draggable)
const totalColumnCount = computed(
  () =>
    visibleColumnCount.value +
    (hasGutter.value ? 1 : 0) +
    (props.columnVisibility ? 1 : 0)
)

// ──────────────────────────────────────────────────────────────────
// Table minimum width — sum of resolved column widths so horizontal
// scroll triggers correctly when columns are wider than viewport.
// ──────────────────────────────────────────────────────────────────
const tableMinWidth = computed(() => {
  let width = 0
  if (hasGutter.value) width += 40
  if (props.columnVisibility) width += 40
  for (const col of table.getVisibleLeafColumns()) {
    width += resolvedColumnWidth(col)
  }
  return `${Math.max(width, 720)}px`
})

function resolvedColumnWidth(col) {
  const sized = columnSizing.value[col.id]
  if (sized) return sized
  const fromMeta = col.columnDef.meta?.width
  if (typeof fromMeta === 'number') return fromMeta
  if (typeof fromMeta === 'string' && fromMeta.endsWith('px')) {
    return parseInt(fromMeta, 10) || 160
  }
  return col.columnDef.size || 160
}

function headerStyle(header) {
  const width = `${resolvedColumnWidth(header.column)}px`
  return { width, minWidth: width, maxWidth: width }
}

function cellStyle(cell) {
  const width = `${resolvedColumnWidth(cell.column)}px`
  return { width, minWidth: width, maxWidth: width }
}

// ──────────────────────────────────────────────────────────────────
// Select-all
// ──────────────────────────────────────────────────────────────────
const selectAllState = computed(() => {
  const total = rows.value.length
  if (total === 0) return false
  const sel = rows.value.filter((r) => r.getIsSelected()).length
  if (sel === 0) return false
  if (sel === total) return true
  return 'indeterminate'
})
function onSelectAll(v) {
  if (v === true) {
    const next = {}
    for (const r of rows.value) next[r.id] = true
    rowSelectionState.value = next
    emit('update:selected', Object.keys(next))
  } else {
    rowSelectionState.value = {}
    emit('update:selected', [])
  }
}

// ──────────────────────────────────────────────────────────────────
// Column drag — threads one __gutter__ placeholder + one __picker__
// placeholder into the draggable id list so SortableJS's DOM index
// → array index mapping stays aligned. Placeholders are stripped
// before pushing the new order back to TanStack.
// ──────────────────────────────────────────────────────────────────
const FIXED_GUTTER_ID = '__gutter__'
const FIXED_PICKER_ID = '__picker__'
const headerRowComponent = ref(null)
const headerRowEl = computed(
  () => headerRowComponent.value?.$el ?? null
)

const draggableColumnIds = ref([])
watch(
  () =>
    [
      table.getAllLeafColumns().map((c) => c.id).join(','),
      props.draggable,
      props.selectable,
      props.columnVisibility
    ].join('||'),
  () => {
    const ids = []
    if (hasGutter.value) ids.push(FIXED_GUTTER_ID)
    ids.push(...table.getAllLeafColumns().map((c) => c.id))
    if (props.columnVisibility) ids.push(FIXED_PICKER_ID)
    draggableColumnIds.value = ids
  },
  { immediate: true }
)

useDraggable(headerRowEl, draggableColumnIds, {
  animation: 150,
  handle: '.column-drag-handle',
  filter: '.no-drag',
  preventOnFilter: true,
  onMove: (event) => !event.related?.classList?.contains('no-drag'),
  onUpdate: () => {
    table.setColumnOrder(
      draggableColumnIds.value.filter(
        (id) => id !== FIXED_GUTTER_ID && id !== FIXED_PICKER_ID
      )
    )
  }
})

// ──────────────────────────────────────────────────────────────────
// Persistence: visibility, order, widths, sorting under storageKey.
// ──────────────────────────────────────────────────────────────────
const STORAGE_PREFIX = 'datatable'
const storage = (suffix) =>
  props.storageKey ? `${STORAGE_PREFIX}:${props.storageKey}:${suffix}` : ''

let suppressSave = true

function readJSON(key) {
  if (!key) return null
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch (e) {
    console.warn(`[DataTable] failed to read ${key}:`, e)
    return null
  }
}

function writeJSON(key, value) {
  if (!key || suppressSave) return
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch (e) {
    console.warn(`[DataTable] failed to write ${key}:`, e)
  }
}

onMounted(() => {
  if (props.storageKey) {
    const v = readJSON(storage('visibility'))
    if (v && typeof v === 'object') columnVisibilityState.value = v

    const o = readJSON(storage('order'))
    if (Array.isArray(o)) columnOrder.value = o

    const w = readJSON(storage('sizes'))
    if (w && typeof w === 'object') columnSizing.value = w

    const s = readJSON(storage('sort'))
    if (Array.isArray(s)) sorting.value = s
  }
  // Allow writes after initial restore tick.
  setTimeout(() => { suppressSave = false }, 0)
})

watch(columnVisibilityState, (v) => writeJSON(storage('visibility'), v), { deep: true })
watch(columnOrder,           (v) => writeJSON(storage('order'),       v), { deep: true })
watch(columnSizing,          (v) => writeJSON(storage('sizes'),       v), { deep: true })
watch(sorting,               (v) => writeJSON(storage('sort'),        v), { deep: true })

const hideableColumns = computed(() =>
  table.getAllLeafColumns().filter((c) => c.getCanHide())
)
function columnLabel(col) {
  if (typeof col.columnDef.header === 'string') return col.columnDef.header
  return col.columnDef.meta?.label || col.id
}

// ──────────────────────────────────────────────────────────────────
// Virtualization (off in drag mode)
// ──────────────────────────────────────────────────────────────────
const scrollContainer = ref(null)

const rowVirtualizer = useVirtualizer({
  get count() { return props.draggable ? 0 : rows.value.length },
  getScrollElement: () => scrollContainer.value,
  estimateSize: () => props.estimatedRowHeight,
  overscan: 10
})

const virtualRows = computed(() =>
  props.draggable ? [] : rowVirtualizer.value.getVirtualItems()
)
const totalSize = computed(() => rowVirtualizer.value.getTotalSize())
const paddingTop = computed(() => virtualRows.value[0]?.start ?? 0)
const paddingBottom = computed(() => {
  const last = virtualRows.value[virtualRows.value.length - 1]
  return last ? totalSize.value - last.end : 0
})

// ──────────────────────────────────────────────────────────────────
// Unified row iteration. Drag mode iterates all rows so SortableJS
// can see every <tr>; virtualized mode iterates the visible window.
// ──────────────────────────────────────────────────────────────────
const renderedRows = computed(() =>
  props.draggable
    ? rows.value
    : virtualRows.value.map((v) => rows.value[v.index])
)

// ──────────────────────────────────────────────────────────────────
// Body container bindings. v-bind="{}" when not draggable so the
// plain TableBody isn't passed any spurious props.
// ──────────────────────────────────────────────────────────────────
const bodyBindings = computed(() =>
  props.draggable
    ? {
        modelValue: internalData.value,
        'onUpdate:modelValue': (v) => (internalData.value = v),
        tag: 'tbody',
        handle: '.datatable-drag-handle',
        animation: 150,
        ghostClass: 'datatable-row-ghost',
        onEnd: onDragEnd
      }
    : {}
)

function onDragEnd(event) {
  const { oldIndex, newIndex } = event
  if (oldIndex == null || newIndex == null || oldIndex === newIndex) return
  emit('reorder', oldIndex, newIndex, internalData.value[newIndex])
}

// ──────────────────────────────────────────────────────────────────
// Row expansion + click
// ──────────────────────────────────────────────────────────────────
const expandedSet = ref(new Set())
function onRowClick(row) {
  if (props.expandable) {
    const next = new Set(expandedSet.value)
    next.has(row.id) ? next.delete(row.id) : next.add(row.id)
    expandedSet.value = next
  }
  emit('row-click', row.original)
}

// ──────────────────────────────────────────────────────────────────
// Visual helpers
// ──────────────────────────────────────────────────────────────────
function alignClass(align) {
  if (align === 'right') return 'text-right'
  if (align === 'center') return 'text-center'
  return 'text-left'
}
function justifyClass(align) {
  if (align === 'right') return 'justify-end'
  if (align === 'center') return 'justify-center'
  return 'justify-start'
}
function rowClass() {
  return [
    'group cursor-pointer border-b border-border/40 transition-colors hover:bg-muted/40 data-[state=selected]:bg-muted/60',
    props.density === 'compact' ? 'py-1' : ''
  ]
}

const emptyTextResolved = computed(
  () => props.emptyText || t('globals.messages.noResultsFound')
)
</script>

<style scoped>
.datatable-row-ghost {
  opacity: 0.4;
  background-color: hsl(var(--muted));
}
</style>
