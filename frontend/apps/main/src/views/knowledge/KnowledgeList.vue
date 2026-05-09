<template>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <div class="flex flex-col h-full">
      <!-- Sub-header strip: description + primary action -->
      <div class="flex items-center justify-between px-6 pt-4 pb-3">
        <p class="text-sm text-muted-foreground">{{ t('admin.knowledge.help') }}</p>
        <Button @click="onNewArticle">{{ t('admin.knowledge.actions.newArticle') }}</Button>
      </div>
      <Separator />

      <!-- Filter bar -->
      <div class="flex items-center gap-2 px-6 py-3">
        <div class="relative flex-1 max-w-xs">
          <Search class="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
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
      </div>

      <!-- Article table -->
      <div class="flex-1 overflow-auto px-6 pb-6">
        <DataTable
          :columns="columns"
          :data="articles"
          :loading="isLoading"
          :searchable="false"
        />
      </div>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Search } from 'lucide-vue-next'
import DataTable from '@main/components/datatable/DataTable.vue'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { Input } from '@shared-ui/components/ui/input/index.js'
import { Separator } from '@shared-ui/components/ui/separator/index.js'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select/index.js'
import { createColumns } from '@/features/knowledge/dataTableColumns.js'
import { useKnowledgeStore } from '@main/stores/knowledge'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
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

const columns = computed(() =>
  createColumns(t, {
    onEdit: (article) =>
      router.push({ name: 'edit-article', params: { id: article.id } }),
    folderById: kb.folderById
  })
)

const buildArticleParams = () => {
  const params = {}
  if (kb.selectedFolderId !== null) params.folder_id = kb.selectedFolderId
  if (statusFilter.value !== 'all') params.status = statusFilter.value
  if (localeFilter.value !== 'all') params.locale = localeFilter.value
  if (searchInput.value.trim()) params.search = searchInput.value.trim()
  return params
}

const fetchArticles = async () => {
  const resp = await api.getKnowledgeArticles(buildArticleParams())
  articles.value = resp.data.data || []
}

const refreshAll = async () => {
  isLoading.value = true
  try {
    await Promise.all([kb.fetchFolders(), fetchArticles()])
  } finally {
    isLoading.value = false
  }
}

// Re-fetch on selected-folder / status / locale change.
watch(() => [kb.selectedFolderId, statusFilter.value, localeFilter.value], fetchArticles)

// Debounce search.
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
</script>
