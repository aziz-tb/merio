<!--
  KnowledgeList — articles table.

  Layout:
    Search row:     external search + status filter + locale filter,
                    + New article button. Stable, never morphs.
    Items bar:      "N items" + columns toggle when no selection.
                    "N selected" + Edit/Publish/Unpublish/Move/Delete
                    + Cancel when rows are selected. Single-row
                    actions (Edit, Publish/Unpublish) show only when
                    exactly one row is selected; multi-row (Move,
                    Delete) show in both cases.
    Table:          Title (link), Folder, Language, Status,
                    Classification, Updated at.

  Drag-to-reorder is enabled only inside a specific folder. UI-only
  until api.reorderKnowledgeArticle exists on the backend.

  Column visibility persists per-browser via the storage-key prop.
-->
<template>
  <div class="flex flex-col h-full">
    <PageHeader />
    <div class="flex-1 overflow-auto">
      <DataTable
        :columns="columns"
        :data="articles"
        :loading="isLoading"
        :searchable="false"
        :selectable="true"
        :draggable="canDrag"
        :column-visibility="true"
        :column-draggable="true"
        storage-key="knowledge-articles"
        v-model:selected="selectedIds"
        :empty-text="t('admin.knowledge.list.empty')"
        @reorder="onReorder"
      >
        <!-- Search + filter row -->
        <template #toolbar>
          <div class="relative max-w-xs flex-1">
            <Search
              class="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              v-model="searchInput"
              :placeholder="t('globals.terms.search')"
              class="pl-8"
            />
          </div>
          <Select v-model="statusFilter">
            <SelectTrigger class="w-[140px]">
              <SelectValue :placeholder="t('globals.terms.status')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ t('globals.messages.all') }}</SelectItem>
              <SelectItem value="draft">{{ t('admin.knowledge.status.draft') }}</SelectItem>
              <SelectItem value="published">{{ t('admin.knowledge.status.published') }}</SelectItem>
            </SelectContent>
          </Select>
          <Select v-model="localeFilter">
            <SelectTrigger class="w-[120px]">
              <SelectValue :placeholder="t('admin.knowledge.locale.label')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ t('globals.messages.all') }}</SelectItem>
              <SelectItem value="en">EN</SelectItem>
              <SelectItem value="de">DE</SelectItem>
              <SelectItem value="fr">FR</SelectItem>
            </SelectContent>
          </Select>
        </template>

        <!-- Primary action: New article. Stays visible regardless of selection. -->
        <template #toolbar-end>
          <Button @click="onNewArticle">
            {{ t('admin.knowledge.actions.newArticle') }}
          </Button>
        </template>

        <!--
          Selected-actions: the contextual action surface in the
          items bar. Single-row actions (Edit, Publish/Unpublish)
          render only when exactly one row is selected; Move and
          Delete render in both single and multi cases.
        -->
        <template #selected-actions="{ selected, clear }">
          <template v-if="selected.length === 1">
            <Button variant="secondary" size="sm" @click="onEdit(selected[0])">
              <Pencil class="w-3.5 h-3.5" />
              {{ t('globals.messages.edit') }}
            </Button>
            <Button
              v-if="selected[0].status === 'draft'"
              variant="secondary"
              size="sm"
              @click="onPublish(selected[0])"
            >
              <Send class="w-3.5 h-3.5" />
              {{ t('admin.knowledge.actions.publish') }}
            </Button>
            <Button
              v-else
              variant="secondary"
              size="sm"
              @click="onUnpublish(selected[0])"
            >
              <EyeOff class="w-3.5 h-3.5" />
              {{ t('admin.knowledge.actions.unpublish') }}
            </Button>
          </template>

          <DropdownMenu>
            <DropdownMenuTrigger
            >
            <Button
              variant="secondary"
              size="sm"
            >
              <FolderInput class="w-3.5 h-3.5" />
              {{ t('admin.knowledge.actions.moveToFolder') }}
            </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" class="max-h-72 overflow-auto">
              <DropdownMenuItem
                v-for="folder in kb.folders"
                :key="folder.id"
                @select="() => onMove(selected, folder.id, clear)"
              >
                {{ folder.name }}
              </DropdownMenuItem>
              <DropdownMenuItem v-if="!kb.folders.length" disabled>
                {{ t('admin.knowledge.folder.empty') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <Button
            variant="destructive"
            size="sm"
            @click="openDeleteConfirm(selected, clear)"
          >
            <Trash2 class="w-3.5 h-3.5" />
            {{ t('globals.messages.delete') }}
          </Button>
        </template>
      </DataTable>
    </div>

    <!-- Delete confirmation (used for both single-row and bulk). -->
    <AlertDialog v-model:open="deleteConfirmOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {{ t('globals.messages.areYouAbsolutelySure') }}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {{
              t(
                'admin.knowledge.bulkDeleteConfirmation',
                pendingDelete.length,
                { n: pendingDelete.length }
              )
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel @click="deleteConfirmOpen = false">
            {{ t('globals.messages.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click="confirmDelete"
          >
            {{ t('globals.messages.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MagnifyingGlass as Search, Pencil, PaperPlane as Send, EyeSlash as EyeOff, FolderArrowDown as FolderInput, TrashCan as Trash2 } from '@qall/icons'
import DataTable from '@main/components/datatable/DataTable.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { Input } from '@shared-ui/components/ui/input/index.js'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@shared-ui/components/ui/select/index.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared-ui/components/ui/dropdown-menu/index.js'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared-ui/components/ui/alert-dialog/index.js'
import { createColumns } from '@/features/knowledge/dataTableColumns.js'
import { useKnowledgeStore } from '@main/stores/knowledge'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api/index.js'

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const kb = useKnowledgeStore()

const isLoading = ref(false)
const articles = ref([])
const searchInput = ref('')
const statusFilter = ref('all')
const localeFilter = ref('all')

const selectedIds = ref([])
const deleteConfirmOpen = ref(false)
const pendingDelete = ref([])
const pendingClear = ref(null)

// Drag is only meaningful inside a specific folder — order across
// folders has no canonical meaning. In "All articles" view, drag is
// disabled to avoid surprising the user.
const canDrag = computed(() => kb.selectedFolderId !== null)

const columns = computed(() =>
  createColumns(t, {
    folderById: kb.folderById,
  })
)

// ── Fetch ─────────────────────────────────────────────────────────
const buildArticleParams = () => {
  const params = {}
  if (kb.selectedFolderId !== null) params.folder_id = kb.selectedFolderId
  if (statusFilter.value !== 'all') params.status = statusFilter.value
  if (localeFilter.value !== 'all') params.locale = localeFilter.value
  if (searchInput.value.trim()) params.search = searchInput.value.trim()
  return params
}

const fetchArticles = async () => {
  isLoading.value = true
  try {
    const resp = await api.getKnowledgeArticles(buildArticleParams())
    articles.value = resp.data.data || []
  } finally {
    isLoading.value = false
  }
}

const refreshAll = async () => {
  isLoading.value = true
  try {
    await Promise.all([kb.fetchFolders(), fetchArticles()])
  } finally {
    isLoading.value = false
  }
}

watch(
  () => [kb.selectedFolderId, statusFilter.value, localeFilter.value],
  () => fetchArticles()
)

let searchTimer = null
watch(searchInput, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => fetchArticles(), 300)
})

const refreshHandler = (data) => {
  if (data?.model === 'knowledge-articles') fetchArticles()
  if (data?.model === 'knowledge-folders') kb.fetchFolders()
}

onMounted(() => {
  refreshAll()
  emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
})
onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  if (searchTimer) clearTimeout(searchTimer)
})

const onNewArticle = () => router.push({ name: 'new-article' })

// ── Single-row actions ────────────────────────────────────────────
function onEdit(article) {
  router.push({ name: 'edit-article', params: { id: article.id } })
}

async function onPublish(article) {
  try {
    await api.publishKnowledgeArticle(article.id)
    fetchArticles()
  } catch (e) {
    showError(e)
  }
}

async function onUnpublish(article) {
  try {
    await api.unpublishKnowledgeArticle(article.id)
    fetchArticles()
  } catch (e) {
    showError(e)
  }
}

// ── Multi-compatible actions (1 or more selected) ─────────────────
async function onMove(selected, folderId, clear) {
  if (!selected.length) return
  try {
    await Promise.all(
      selected.map((a) =>
        api.moveKnowledgeArticle(a.id, { folder_id: folderId })
      )
    )
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.knowledge.actions.movedSuccessfully'),
    })
    clear?.()
    fetchArticles()
  } catch (e) {
    showError(e)
  }
}

function openDeleteConfirm(selected, clear) {
  pendingDelete.value = selected
  pendingClear.value = clear
  deleteConfirmOpen.value = true
}

async function confirmDelete() {
  const items = pendingDelete.value
  const clear = pendingClear.value
  deleteConfirmOpen.value = false
  if (!items.length) return
  try {
    await Promise.all(items.map((a) => api.deleteKnowledgeArticle(a.id)))
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.deletedSuccessfully'),
    })
    clear?.()
    fetchArticles()
  } catch (e) {
    showError(e)
  }
}

// ── Row drag reorder ──────────────────────────────────────────────
async function onReorder(fromIndex, toIndex, item) {
  // UI-only until api.reorderKnowledgeArticle exists. When the
  // endpoint lands, this branch starts persisting automatically.
  if (typeof api.reorderKnowledgeArticle !== 'function') {
    console.warn(
      '[KnowledgeList] api.reorderKnowledgeArticle not implemented — drag is UI-only.'
    )
    return
  }
  try {
    await api.reorderKnowledgeArticle(item.id, {
      folder_id: kb.selectedFolderId,
      position: toIndex,
    })
  } catch (e) {
    fetchArticles()
    showError(e)
  }
}

function showError(e) {
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
    variant: 'destructive',
    description: handleHTTPError(e).message,
  })
}
</script>
