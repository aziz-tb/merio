<template>
  <div class="space-y-2">
    <p class="text-xs text-muted-foreground">
      {{ $t('admin.ai.agent.kbScope.help') }}
    </p>
    <div v-if="loading" class="text-xs text-muted-foreground">
      {{ $t('globals.messages.loading') }}
    </div>
    <ul v-else class="space-y-1">
      <KBScopeNode
        v-for="f in tree"
        :key="`f-${f.id}`"
        :folder="f"
        :folder-articles="folderArticles"
        :selected-folder-ids="selectedFolderIds"
        :selected-article-ids="selectedArticleIds"
        :load-articles="loadArticles"
        @toggle-folder="toggleFolder"
        @toggle-article="toggleArticle"
      />
    </ul>
  </div>
</template>

<script setup>
// KBScopePicker — folder + article checkbox tree. Folders nest by
// parent_id; expanding a folder lazy-loads its published articles.
// v-model carries { folder_ids: number[], article_ids: number[] }.
//
// The picker emits whitelist additions and removals; the parent owns
// the canonical state. Article toggles are additive — kb_search uses
// folder_ids as the scope ceiling and article_ids as the explicit
// whitelist on top.
import { ref, computed, onMounted } from 'vue'
import api from '@/api'
import KBScopeNode from '@/features/ai/agents/form/KbScopeNode.vue'

const props = defineProps({
  modelValue: {
    type: Object,
    default: () => ({ folder_ids: [], article_ids: [] })
  }
})
const emit = defineEmits(['update:modelValue'])

const loading = ref(true)
const folders = ref([])
const folderArticles = ref({}) // folder id → Article[] (lazy)

const selectedFolderIds = computed(() => new Set((props.modelValue.folder_ids || []).map(Number)))
const selectedArticleIds = computed(() => new Set((props.modelValue.article_ids || []).map(Number)))

const tree = computed(() => {
  const byParent = new Map()
  for (const f of folders.value) {
    const k = f.parent_id ?? null
    if (!byParent.has(k)) byParent.set(k, [])
    byParent.get(k).push(f)
  }
  const decorate = (list) => {
    return (list || [])
      .slice()
      .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name))
      .map((f) => ({ ...f, children: decorate(byParent.get(f.id)) }))
  }
  return decorate(byParent.get(null))
})

const emitChange = (folderIds, articleIds) => {
  emit('update:modelValue', { folder_ids: [...folderIds], article_ids: [...articleIds] })
}

const toggleFolder = ({ id, on }) => {
  const next = new Set(selectedFolderIds.value)
  if (on) next.add(id)
  else next.delete(id)
  emitChange(next, selectedArticleIds.value)
}

const toggleArticle = ({ id, on }) => {
  const next = new Set(selectedArticleIds.value)
  if (on) next.add(id)
  else next.delete(id)
  emitChange(selectedFolderIds.value, next)
}

const loadArticles = async (folderId) => {
  if (folderArticles.value[folderId]) return
  try {
    const r = await api.getKnowledgeArticles({ folder_id: folderId, page_size: 200 })
    const data = r.data.data
    const list = Array.isArray(data) ? data : data?.results || []
    folderArticles.value = { ...folderArticles.value, [folderId]: list }
  } catch {
    folderArticles.value = { ...folderArticles.value, [folderId]: [] }
  }
}

onMounted(async () => {
  try {
    const r = await api.getKnowledgeFolders()
    folders.value = r.data.data || []
  } catch {
    folders.value = []
  } finally {
    loading.value = false
  }
})
</script>
