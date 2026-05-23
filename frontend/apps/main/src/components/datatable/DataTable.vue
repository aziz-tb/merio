<!--
  DataTable — unified list/table for the application.

  Toolbar (stable):    search + #toolbar + #toolbar-end
  Items bar (morphs):  "N items" / "N selected" + #selected-actions + cancel;
                       #header-actions + columns toggle on the right
  Table region:        sticky header, sortable + column-draggable headers,
                       virtualized body when not in row-drag mode

  Opt-in via props:
    - selectable + v-model:selected     row checkboxes + select-all
    - draggable + @reorder              row drag via handle
    - columnVisibility                  columns toggle in items bar
    - columnDraggable                   drag column headers to reorder
    - storageKey                        persists column visibility under
                                        `datatable:<storageKey>:visibility`
    - pagination='page'|'cursor'        bottom pagination
    - expandable + #expanded-row        per-row expansion

  Slots:
    - #toolbar / #toolbar-end           search-row controls
    - #header-actions                   icons left of the columns toggle
    - #selected-actions={ selected, clear }
                                        shown when rows are selected
    - #expanded-row={ row }             content under an expanded row
    - #empty                            custom empty state
-->
<template>
  <LoadingOverlay :loading="loading" reserve-height>
    <div class="w-full space-y-3">
      <!-- Toolbar -->
      <div class="flex items-center gap-3 min-h-9">
        <div v-if="searchable" class="relative max-w-xs flex-1">
          <Search
            class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            v-model="globalFilter"
            :placeholder="searchPlaceholder || t('globals.terms.search')"
            class="pl-8"
          />
        </div>
        <div class="flex items-center gap-2 flex-1">
          <slot name="toolbar" />
        </div>
        <div class="flex items-center gap-2 ml-auto shrink-0">
          <slot name="toolbar-end" />
        </div>
      </div>

      <!-- Items bar -->
      <div
        v-if="rows.length > 0"
        class="flex items-center justify-between min-h-9 px-1"
      >
        <div class="flex items-center gap-3 min-w-0 flex-wrap">
          <span class="text-sm font-medium text-foreground whitespace-nowrap">
            <template v-if="selectedCount === 0">
              {{ t('datatable.itemCount', rows.length, { n: rows.length }) }}
            </template>
            <template v-else>
              {{ t('datatable.itemSelected', selectedCount, { n: selectedCount }) }}
            </template>
          </span>
          <div v-if="selectedCount > 0" class="flex items-center gap-2 flex-wrap">
            <slot
              name="selected-actions"
              :selected="selectedRowData"
              :clear="clearSelection"
            />
            <Button variant="ghost" size="sm" @click="clearSelection">
              {{ t('globals.messages.cancel') }}
            </Button>
          </div>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <slot name="header-actions" />
          <DropdownMenu v-if="columnVisibility">
            <DropdownMenuTrigger
              class="inline-flex items-center justify-center w-8 h-8 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
              :title="t('datatable.columns')"
            >
              <Columns class="w-4 h-4" />
              <span class="sr-only">{{ t('datatable.columns') }}</span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" class="w-48">
              <DropdownMenuLabel>{{ t('datatable.columns') }}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuCheckboxItem
                v-for="col in hideableColumns"
                :key="col.id"
                :checked="col.getIsVisible()"
                @update:checked="(v) => col.toggleVisibility(!!v)"
              >
                {{ columnLabel(col) }}
              </DropdownMenuCheckboxItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <!-- Table region -->
      <div class="border-b border-input overflow-hidden">
        <div
          ref="scrollContainer"
          class="relative overflow-auto"
          :style="{ maxHeight }"
        >
          <table class="w-full caption-bottom text-sm -ml-4">
            <!-- Header -->
            <TableHeader class="sticky top-0 z-10 bg-background">
              <TableRow
                ref="headerRowComponent"
                class="group border-b border-border hover:bg-transparent"
              >
                <!-- Gutter (normal first cell, holds select-all) -->
                <TableHead v-if="hasGutter" class="no-drag w-12">
                  <div class="flex items-center justify-center pt-1 pl-4">
                    <Checkbox
                      v-if="selectable"
                      :checked="selectAllState"
                      @update:checked="onSelectAll"
                      :aria-label="t('datatable.selectAll')"
                      :class="[
                        'transition-opacity',
                        selectedCount === 0
                          ? 'opacity-0 group-hover:opacity-100'
                          : 'opacity-100',
                      ]"
                    />
                  </div>
                </TableHead>

                <!-- Data column headers -->
                <TableHead
                  v-for="header in headerCells"
                  :key="header.id"
                  :class="[
                    'relative h-11 px-4 text-sm font-medium text-muted-foreground',
                    alignClass(header.column.columnDef.meta?.align),
                    header.column.getCanSort() &&
                      'cursor-pointer select-none transition-colors hover:text-foreground',
                    header.column.columnDef.meta?.isActions && 'no-drag',
                  ]"
                  :style="sizeStyle(header.column.columnDef.meta?.width)"
                  @click="header.column.getToggleSortingHandler()?.($event)"
                >
                  <Ellipsis
                    v-if="columnDraggable && !header.column.columnDef.meta?.isActions"
                    class="column-drag-handle absolute top-0 left-1/2 -translate-x-1/2 -translate-y-1/3 w-5 h-5 p-0.5 text-muted-foreground bg-background opacity-0 group-hover:opacity-100 transition-opacity cursor-grab active:cursor-grabbing z-20"
                    :title="t('datatable.dragToReorder')"
                    @click.stop
                  />
                  <div
                    :class="[
                      'flex items-center gap-2',
                      justifyClass(header.column.columnDef.meta?.align),
                    ]"
                  >
                    <FlexRender
                      v-if="!header.isPlaceholder"
                      :render="header.column.columnDef.header"
                      :props="header.getContext()"
                    />
                    <template v-if="header.column.getCanSort()">
                      <ChevronUp
                        v-if="header.column.getIsSorted() === 'asc'"
                        class="h-3.5 w-3.5"
                      />
                      <ChevronDown
                        v-else-if="header.column.getIsSorted() === 'desc'"
                        class="h-3.5 w-3.5"
                      />
                      <ArrowUpDown
                        v-else
                        class="h-3.5 w-3.5 opacity-0 transition-opacity"
                      />
                    </template>
                  </div>
                </TableHead>
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
                  <TableCell v-if="hasGutter" class="w-12">
                    <div class="flex items-center justify-center gap-1">
                      <EllipsisVertical
                        v-if="draggable"
                        class="datatable-drag-handle w-3.5 h-3.5 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity cursor-grab active:cursor-grabbing shrink-0"
                        :title="t('datatable.dragToReorder')"
                      />
                      <Checkbox
                        v-if="selectable"
                        :checked="row.getIsSelected()"
                        @update:checked="(v) => row.toggleSelected(!!v)"
                        @click.stop
                        :class="[
                          'transition-opacity shrink-0',
                          row.getIsSelected()
                            ? 'opacity-100'
                            : 'opacity-0 group-hover:opacity-100',
                        ]"
                      />
                    </div>
                  </TableCell>

                  <!-- Data cells -->
                  <TableCell
                    v-for="cell in row.getVisibleCells()"
                    :key="cell.id"
                    :class="[
                      'px-4 py-3 text-sm',
                      alignClass(cell.column.columnDef.meta?.align),
                      cell.column.columnDef.meta?.font === 'mono' && 'font-mono',
                    ]"
                    :style="sizeStyle(cell.column.columnDef.meta?.width)"
                  >
                    <FlexRender
                      :render="cell.column.columnDef.cell"
                      :props="cell.getContext()"
                    />
                  </TableCell>
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
                <TableCell :colspan="totalColumnCount" class="h-32 px-4">
                  <slot name="empty">
                    <div class="flex flex-col items-center justify-center gap-2 text-center py-6">
                      <component :is="emptyIcon" class="h-8 w-8 text-muted-foreground/50" />
                      <p class="text-sm font-medium text-muted-foreground">
                        {{ emptyTextResolved }}
                      </p>
                    </div>
                  </slot>
                </TableCell>
              </TableRow>
            </TableBody>
          </table>
        </div>
      </div>

      <!-- Pagination -->
      <PaginationBar
        v-if="pagination === 'page'"
        :page="page"
        :per-page="perPage"
        :total-pages="totalPages"
        :per-page-options="perPageOptions"
        @update:page="(v) => emit('update:page', v)"
        @update:per-page="(v) => emit('update:perPage', v)"
      />
      <div
        v-else-if="pagination === 'cursor' && hasMore"
        class="flex justify-center py-2"
      >
        <button
          type="button"
          class="text-xs text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
          :disabled="loading"
          @click="emit('load-more')"
        >
          {{
            loading
              ? t('globals.messages.loading')
              : t('globals.messages.loadOlder')
          }}
        </button>
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
  useVueTable,
} from '@tanstack/vue-table'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { VueDraggable, useDraggable } from 'vue-draggable-plus'
import { useI18n } from 'vue-i18n'
import {
  ArrowUpDown,
  ChevronDown,
  ChevronUp,
  Columns,
  Ghost,
  Ellipsis,
  EllipsisVertical,
  Search,
} from 'lucide-vue-next'
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@shared-ui/components/ui/table'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
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

  searchable: { type: Boolean, default: true },
  searchPlaceholder: { type: String, default: '' },
  maxHeight: { type: String, default: '70vh' },
  density: {
    type: String,
    default: 'comfortable',
    validator: (v) => ['comfortable', 'compact'].includes(v),
  },
  estimatedRowHeight: { type: Number, default: 52 },

  selectable: { type: Boolean, default: false },
  selected: { type: Array, default: () => [] },
  draggable: { type: Boolean, default: false },
  columnVisibility: { type: Boolean, default: false },
  columnDraggable: { type: Boolean, default: false },
  storageKey: { type: String, default: '' },

  pagination: {
    type: String,
    default: 'none',
    validator: (v) => ['none', 'page', 'cursor'].includes(v),
  },
  page: { type: Number, default: 1 },
  perPage: { type: Number, default: 25 },
  totalPages: { type: Number, default: 1 },
  perPageOptions: { type: Array, default: () => [15, 30, 50, 100] },
  hasMore: { type: Boolean, default: false },

  expandable: { type: Boolean, default: false },
})

const emit = defineEmits([
  'update:selected',
  'reorder',
  'update:page',
  'update:perPage',
  'load-more',
  'row-click',
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

const table = useVueTable({
  get data() { return internalData.value },
  get columns() { return props.columns },
  state: {
    get sorting() { return sorting.value },
    get globalFilter() { return globalFilter.value },
    get rowSelection() { return rowSelectionState.value },
    get columnVisibility() { return columnVisibilityState.value },
    get columnOrder() { return columnOrder.value },
  },
  enableSortingRemoval: false,
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
  getCoreRowModel: getCoreRowModel(),
  getSortedRowModel: getSortedRowModel(),
  getFilteredRowModel: getFilteredRowModel(),
})

const rows = computed(() => table.getRowModel().rows)
const headerCells = computed(
  () => table.getHeaderGroups()[0]?.headers ?? []
)

// ──────────────────────────────────────────────────────────────────
// Gutter
// ──────────────────────────────────────────────────────────────────
const hasGutter = computed(() => props.selectable || props.draggable)
const totalColumnCount = computed(
  () => props.columns.length + (hasGutter.value ? 1 : 0)
)

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
// Column drag — threads one __gutter__ placeholder into the
// draggable id list when a gutter exists, so SortableJS's DOM
// index → array index mapping stays aligned. The placeholder is
// stripped before pushing the new order back to TanStack.
// ──────────────────────────────────────────────────────────────────
const FIXED_GUTTER_ID = '__gutter__'
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
    ].join('||'),
  () => {
    const ids = []
    if (hasGutter.value) ids.push(FIXED_GUTTER_ID)
    ids.push(...table.getAllLeafColumns().map((c) => c.id))
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
      draggableColumnIds.value.filter((id) => id !== FIXED_GUTTER_ID)
    )
  },
})

// ──────────────────────────────────────────────────────────────────
// Column visibility persistence
// ──────────────────────────────────────────────────────────────────
const STORAGE_PREFIX = 'datatable'

onMounted(() => {
  if (!props.storageKey) return
  try {
    const raw = localStorage.getItem(
      `${STORAGE_PREFIX}:${props.storageKey}:visibility`
    )
    if (raw) {
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object') {
        columnVisibilityState.value = parsed
      }
    }
  } catch (e) {
    console.warn(
      `[DataTable] failed to load visibility for "${props.storageKey}":`,
      e
    )
  }
})

let visibilitySaveTimer = null
watch(
  columnVisibilityState,
  (state) => {
    if (!props.storageKey) return
    if (visibilitySaveTimer) clearTimeout(visibilitySaveTimer)
    visibilitySaveTimer = setTimeout(() => {
      try {
        localStorage.setItem(
          `${STORAGE_PREFIX}:${props.storageKey}:visibility`,
          JSON.stringify(state)
        )
      } catch (e) {
        console.warn(
          `[DataTable] failed to save visibility for "${props.storageKey}":`,
          e
        )
      }
    }, 200)
  },
  { deep: true }
)

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
  overscan: 10,
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
        onEnd: onDragEnd,
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
  if (align === 'left') return 'text-left'
  if (align === 'right') return 'text-right'
  return 'text-center'
}
function justifyClass(align) {
  if (align === 'left') return 'justify-start'
  if (align === 'right') return 'justify-end'
  return 'justify-center'
}
function sizeStyle(width) {
  return width ? { width, minWidth: width } : {}
}
function rowClass(row) {
  return [
    'group border-b border-border/50 transition-colors last:border-0 hover:bg-muted/30 data-[state=selected]:bg-muted',
    props.expandable && 'cursor-pointer',
    props.density === 'compact' ? 'py-1.5' : 'py-3',
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