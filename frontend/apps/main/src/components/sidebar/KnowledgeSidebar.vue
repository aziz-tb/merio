<!-- Knowledge-mode contents for the secondary sidebar.
     Two sub-modes driven by route name:
       - knowledge-list      : folder tree + "All articles" + "+ New folder"
       - new-article / edit-article : article metadata fields (status, folder,
         slug, locale, classification, AI-grounded). The page itself shows
         only the title input + full-page editor. -->
<template>
  <div class="flex flex-col h-full">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <div class="px-1">
            <span class="text-xs font-normal text-muted-foreground uppercase tracking-wider px-2">
              {{ isFormRoute ? t('admin.knowledge.actions.editArticle') : t('globals.terms.knowledgeBase') }}
            </span>
          </div>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>

    <SidebarContent>
      <!-- ============================================================
           List mode: folder tree
           ============================================================ -->
      <template v-if="isListRoute">
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                :is-active="kb.selectedFolderId === null"
                @click="kb.selectedFolderId = null"
              >
                <span>{{ t('admin.knowledge.allFolders') }}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
        <SidebarGroup class="pt-0">
          <FolderTree
            :folders="kb.folders"
            :selected-id="kb.selectedFolderId"
            :hide-header="true"
            @select="(id) => (kb.selectedFolderId = id)"
            @new-subfolder="onNewSubfolder"
            @rename="onRename"
            @move="onMoveFolder"
            @delete="onDeleteFolder"
          />
        </SidebarGroup>
      </template>

      <!-- ============================================================
           Form mode: article metadata
           ============================================================ -->
      <template v-else-if="isFormRoute && kb.isFormMode">
        <!-- Status -->
        <SidebarGroup>
          <div class="px-2 space-y-2">
            <span class="text-xs font-medium text-muted-foreground uppercase tracking-wider">
              {{ t('globals.terms.status') }}
            </span>
            <div class="flex items-center justify-between gap-2">
              <Badge :variant="kb.articleStatus === 'published' ? 'default' : 'secondary'">
                {{ t(`admin.knowledge.status.${kb.articleStatus}`) }}
              </Badge>
              <Button
                v-if="kb.formMode === 'edit' && kb.articleStatus === 'draft'"
                size="sm"
                @click="onPublish"
              >
                {{ t('admin.knowledge.actions.publish') }}
              </Button>
              <Button
                v-else-if="kb.formMode === 'edit' && kb.articleStatus === 'published'"
                size="sm"
                variant="outline"
                @click="onUnpublish"
              >
                {{ t('admin.knowledge.actions.unpublish') }}
              </Button>
            </div>
          </div>
        </SidebarGroup>

        <Separator />

        <SidebarGroup>
          <div class="px-2 space-y-4">
            <!-- Folder -->
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted-foreground">
                {{ t('globals.terms.folder') }}
              </label>
              <FolderPicker
                :model-value="kb.fields.folder_id ?? null"
                :folders="kb.folders"
                @update:model-value="(v) => (kb.fields.folder_id = v)"
              />
              <p v-if="kb.errors.folder_id" class="text-xs text-destructive">
                {{ kb.errors.folder_id }}
              </p>
            </div>

            <!-- Locale -->
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted-foreground">
                {{ t('admin.knowledge.locale.label') }}
              </label>
              <select
                v-model="kb.fields.locale"
                class="h-9 rounded border border-input bg-transparent px-2 w-full text-sm"
              >
                <option value="en">EN</option>
                <option value="de">DE</option>
                <option value="fr">FR</option>
              </select>
              <p v-if="kb.errors.locale" class="text-xs text-destructive">
                {{ kb.errors.locale }}
              </p>
            </div>

            <!-- Classification -->
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted-foreground">
                {{ t('admin.knowledge.classification.label') }}
              </label>
              <select
                v-model="kb.fields.classification"
                class="h-9 rounded border border-input bg-transparent px-2 w-full text-sm"
              >
                <option value="faq">{{ t('admin.knowledge.classification.faq') }}</option>
                <option value="how_to">{{ t('admin.knowledge.classification.how_to') }}</option>
                <option value="policy">{{ t('admin.knowledge.classification.policy') }}</option>
                <option value="news">{{ t('admin.knowledge.classification.news') }}</option>
                <option value="other">{{ t('admin.knowledge.classification.other') }}</option>
              </select>
              <p v-if="kb.errors.classification" class="text-xs text-destructive">
                {{ kb.errors.classification }}
              </p>
            </div>

            <!-- AI groundable -->
            <div class="flex items-start gap-3 pt-1">
              <input
                type="checkbox"
                class="mt-1"
                :checked="kb.fields.is_ai_groundable"
                @change="kb.fields.is_ai_groundable = $event.target.checked"
              />
              <div class="space-y-1">
                <label class="text-xs font-medium">{{ t('admin.knowledge.aiGroundable.label') }}</label>
                <p class="text-xs text-muted-foreground">
                  {{ t('admin.knowledge.aiGroundable.description') }}
                </p>
              </div>
            </div>
          </div>
        </SidebarGroup>
      </template>
    </SidebarContent>

    <!-- Footer: list-mode "+ New folder" button -->
    <div v-if="isListRoute" class="border-t border-border p-2">
      <Button variant="outline" size="sm" class="w-full" @click="onNewRoot">
        <Plus class="size-4 mr-1" />
        {{ t('admin.knowledge.actions.newFolder') }}
      </Button>
    </div>
  </div>

  <!-- ====================================================================
       Folder dialogs (only used in list mode)
       ==================================================================== -->
  <FolderDialog
    :open="folderDialog.open"
    :mode="folderDialog.mode"
    :initial="folderDialog.initial"
    @update:open="(v) => (folderDialog.open = v)"
    @submit="onFolderDialogSubmit"
  />

  <Dialog :open="moveFolderDialog.open" @update:open="(v) => (moveFolderDialog.open = v)">
    <DialogContent class="sm:max-w-[425px]">
      <DialogHeader>
        <DialogTitle>{{ t('admin.knowledge.actions.move') }}</DialogTitle>
      </DialogHeader>
      <div class="space-y-4">
        <FolderPicker
          v-model="moveFolderDialog.targetParentId"
          :folders="kb.folders"
          :allow-null="true"
          :exclude-ids="moveExcludeIds"
        />
        <DialogFooter>
          <Button variant="ghost" @click="moveFolderDialog.open = false">
            {{ t('globals.messages.cancel') }}
          </Button>
          <Button @click="confirmMoveFolder">{{ t('globals.messages.save') }}</Button>
        </DialogFooter>
      </div>
    </DialogContent>
  </Dialog>

  <AlertDialog v-model:open="deleteFolderAlert.open">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ t('admin.knowledge.deleteFolderConfirmation') }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="confirmDeleteFolder">
          {{ t('globals.messages.delete') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import {
  SidebarContent,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem
} from '@shared-ui/components/ui/sidebar'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { Badge } from '@shared-ui/components/ui/badge'
import { Separator } from '@shared-ui/components/ui/separator/index.js'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog/index.js'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@shared-ui/components/ui/alert-dialog/index.js'
import FolderTree from '@/features/knowledge/FolderTree.vue'
import FolderDialog from '@/features/knowledge/FolderDialog.vue'
import FolderPicker from '@/features/knowledge/FolderPicker.vue'
import { useKnowledgeStore } from '@main/stores/knowledge'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api/index.js'

const { t } = useI18n()
const route = useRoute()
const emitter = useEmitter()
const kb = useKnowledgeStore()

const isListRoute = computed(() => route.name === 'knowledge-list')
const isFormRoute = computed(() => route.name === 'new-article' || route.name === 'edit-article')

const folderDialog = ref({
  open: false,
  mode: 'create',
  initial: { name: '', slug: '' },
  parentId: null,
  renameId: null
})
const moveFolderDialog = ref({ open: false, folderId: null, targetParentId: null })
const deleteFolderAlert = ref({ open: false, folderId: null })

const moveExcludeIds = computed(() => {
  if (!moveFolderDialog.value.folderId) return []
  const ids = new Set([moveFolderDialog.value.folderId])
  let added = true
  while (added) {
    added = false
    for (const f of kb.folders) {
      if (f.parent_id != null && ids.has(f.parent_id) && !ids.has(f.id)) {
        ids.add(f.id)
        added = true
      }
    }
  }
  return [...ids]
})

const onNewRoot = () => {
  folderDialog.value = {
    open: true,
    mode: 'create',
    initial: { name: '', slug: '' },
    parentId: null,
    renameId: null
  }
}
const onNewSubfolder = (parentId) => {
  folderDialog.value = {
    open: true,
    mode: 'create',
    initial: { name: '', slug: '' },
    parentId,
    renameId: null
  }
}
const onRename = (folder) => {
  folderDialog.value = {
    open: true,
    mode: 'rename',
    initial: { name: folder.name, slug: folder.slug },
    parentId: null,
    renameId: folder.id
  }
}
const onMoveFolder = (folder) => {
  moveFolderDialog.value = {
    open: true,
    folderId: folder.id,
    targetParentId: folder.parent_id ?? null
  }
}
const onDeleteFolder = (folder) => {
  deleteFolderAlert.value = { open: true, folderId: folder.id }
}

const onFolderDialogSubmit = async (values) => {
  try {
    if (folderDialog.value.mode === 'create') {
      await api.createKnowledgeFolder({
        name: values.name,
        slug: values.slug,
        parent_id: folderDialog.value.parentId
      })
    } else {
      await api.updateKnowledgeFolder(folderDialog.value.renameId, {
        name: values.name,
        slug: ''
      })
    }
    folderDialog.value.open = false
    await kb.fetchFolders()
    // Tell the article list to refresh, in case folder name changes affect rows.
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'knowledge-articles' })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

const confirmMoveFolder = async () => {
  try {
    await api.moveKnowledgeFolder(moveFolderDialog.value.folderId, {
      parent_id: moveFolderDialog.value.targetParentId
    })
    moveFolderDialog.value.open = false
    await kb.fetchFolders()
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

const confirmDeleteFolder = async () => {
  try {
    await api.deleteKnowledgeFolder(deleteFolderAlert.value.folderId)
    deleteFolderAlert.value.open = false
    if (kb.selectedFolderId === deleteFolderAlert.value.folderId) {
      kb.selectedFolderId = null
    }
    await kb.fetchFolders()
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'knowledge-articles' })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

// ---------------------------------------------------------------------------
// Form-mode: Publish/Unpublish
// ---------------------------------------------------------------------------
const onPublish = async () => {
  if (!kb.articleId) return
  try {
    const resp = await api.publishKnowledgeArticle(kb.articleId)
    kb.setStatus(resp.data.data.status)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.knowledge.toasts.published')
    })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

const onUnpublish = async () => {
  if (!kb.articleId) return
  try {
    const resp = await api.unpublishKnowledgeArticle(kb.articleId)
    kb.setStatus(resp.data.data.status)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.knowledge.toasts.unpublished')
    })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

// Folders are needed in both list-mode (tree) and form-mode (picker). Fetch
// once when the sidebar mounts into a knowledge route. The list page also
// fetches independently and writes through the same store ref, so the data
// stays consistent.
onMounted(() => {
  if (kb.folders.length === 0) kb.fetchFolders()
})

// Refresh the folder list when an article-list refresh emits — covers
// folder edits triggered from anywhere else in the app.
const refreshHandler = (data) => {
  if (data?.model === 'knowledge-folders') kb.fetchFolders()
}
onMounted(() => emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler))
onUnmounted(() => emitter.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler))
</script>
