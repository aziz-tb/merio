<!--
  AgentTabKnowledge — flat table with folder cascade for KB scope.

  Row types:
    - Folder row: chevron + folder icon + name + scope checkbox.
      Indeterminate when not in folder_ids but ≥1 child article is in
      article_ids.
    - Article row: indented ~24px when its parent folder is expanded.
      Type icon + title + parent folder name + updated_at + scope checkbox.

  Scope semantics (mirrors the backend):
    - folder_ids ⊂ kb_scope: the scope ceiling. Articles under a
      whitelisted folder are in scope automatically.
    - article_ids ⊂ kb_scope: additive whitelist for individual articles
      regardless of folder.
    - "Folder cascade" in the UI is visual only: an article shows as
      "in scope" when (parent_folder ∈ folder_ids) OR (article ∈ article_ids).
      Folder toggles only manage folder_ids; article toggles only
      manage article_ids. Less surprising than a state-rewriting cascade.

  Articles are lazy-loaded per folder on expand (matches the existing
  KBScopePicker behavior + keeps initial render light).

  Reads/writes agent.kb_scope via inject. Search filter narrows to
  matching folders + articles; matching folders auto-expand.
-->
<template>
  <div class="px-6 py-6 max-w-5xl mx-auto">
    <div class="flex items-center justify-between gap-4 mb-3">
      <h2 class="text-lg font-semibold">{{ t('admin.ai.agent.tab.knowledge') }}</h2>
      <span class="text-xs text-muted-foreground">
        {{ inScopeCount }} {{ t('globals.terms.inScope') }}
        <span class="mx-1">·</span>
        {{ totalArticles }} {{ t('globals.terms.total') }}
      </span>
    </div>
    <p class="text-sm text-muted-foreground mb-4">
      {{ t('admin.ai.agent.knowledge.help') }}
    </p>

    <div class="mb-3">
      <Input
        v-model="search"
        type="text"
        :placeholder="t('globals.messages.search')"
        @keydown.enter.prevent
      />
    </div>

    <div v-if="loading" class="text-sm text-muted-foreground">
      {{ t('globals.messages.loading') }}
    </div>
    <div v-else-if="visibleRows.length === 0" class="text-sm text-muted-foreground py-6 text-center border border-dashed border-input rounded-md">
      {{ t('admin.ai.agent.knowledge.empty') }}
    </div>
    <div v-else class="border border-input rounded-md overflow-hidden">
      <!-- Header row -->
      <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,180px)_minmax(0,140px)_40px] gap-3 px-4 py-2 text-xs text-muted-foreground bg-muted/40 border-b border-input">
        <span>{{ t('globals.terms.title') }}</span>
        <span>{{ t('globals.terms.folder') }}</span>
        <span>{{ t('globals.terms.updated') }}</span>
        <span class="text-right">{{ t('globals.terms.scope') }}</span>
      </div>

      <ul class="divide-y divide-input">
        <li
          v-for="row in visibleRows"
          :key="row.key"
          :class="[
            'grid grid-cols-[minmax(0,1fr)_minmax(0,180px)_minmax(0,140px)_40px] gap-3 px-4 py-2 items-center text-sm',
            row.type === 'article' ? 'pl-10' : '',
          ]"
        >
          <!-- Title cell -->
          <div class="flex items-center gap-2 min-w-0">
            <button
              v-if="row.type === 'folder'"
              type="button"
              class="shrink-0 text-muted-foreground hover:text-foreground"
              @click="toggleExpand(row.id)"
            >
              <component :is="expanded.has(row.id) ? ChevronDown : ChevronRight" class="w-4 h-4" />
            </button>
            <component
              :is="row.type === 'folder' ? Folder : FileText"
              class="w-4 h-4 shrink-0 text-muted-foreground"
            />
            <span class="truncate" :title="row.title">{{ row.title }}</span>
          </div>

          <!-- Folder column -->
          <span class="truncate text-xs text-muted-foreground">{{ row.folderLabel }}</span>

          <!-- Updated column -->
          <span class="truncate text-xs text-muted-foreground">{{ row.updated }}</span>

          <!-- Scope checkbox -->
          <div class="flex justify-end">
            <Checkbox
              :checked="row.checkboxState"
              @update:checked="(v) => onCheckboxChange(row, v)"
            />
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronRight, Folder, FileLines as FileText } from '@qall/icons'
import { Input } from '@shared-ui/components/ui/input'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import api from '@/api'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)

const search = ref('')
const loading = ref(true)
const folders = ref([])
const folderArticles = ref({}) // folder.id → Article[] (lazy)
const expanded = ref(new Set())

// Tree of folders by parent. We render flat but compute the tree
// for ordering + breadcrumb-style folder labels.
const folderTree = computed(() => {
  const byParent = new Map()
  for (const f of folders.value) {
    const k = f.parent_id ?? null
    if (!byParent.has(k)) byParent.set(k, [])
    byParent.get(k).push(f)
  }
  const order = (list) =>
    (list || []).slice().sort(
      (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name)
    )
  return order(byParent.get(null)).map((f) => ({
    ...f,
    children: order(byParent.get(f.id)),
  }))
})

const folderById = computed(() => {
  const m = new Map()
  for (const f of folders.value) m.set(f.id, f)
  return m
})

// Reactive selection sets from agent.kb_scope. Writing back constructs
// new arrays so the deep-equal dirty check fires.
const selectedFolderIds = computed(
  () => new Set((agent.value?.kb_scope?.folder_ids || []).map(Number))
)
const selectedArticleIds = computed(
  () => new Set((agent.value?.kb_scope?.article_ids || []).map(Number))
)

async function loadArticles(folderId) {
  if (folderArticles.value[folderId]) return
  try {
    const r = await api.getKnowledgeArticles({ folder_id: folderId, page_size: 500 })
    const data = r.data.data
    const list = Array.isArray(data) ? data : data?.results || []
    folderArticles.value = { ...folderArticles.value, [folderId]: list }
  } catch {
    folderArticles.value = { ...folderArticles.value, [folderId]: [] }
  }
}

async function loadAllFolderArticles() {
  // Eager readback for explicit article_ids. At current KB sizes a
  // fan-out fetch across every folder is cheap; once catalogs grow past
  // a few hundred articles this should move to a dedicated
  // article->folder index endpoint so persisted scope can render
  // without preloading the whole KB.
  await Promise.all((folders.value || []).map((folder) => loadArticles(folder.id)))
}

function toggleExpand(folderId) {
  const next = new Set(expanded.value)
  if (next.has(folderId)) {
    next.delete(folderId)
  } else {
    next.add(folderId)
    loadArticles(folderId)
  }
  expanded.value = next
}

// Visible rows: walks the tree depth-first, emitting one row per
// folder + (when expanded) each of its articles. Search filter
// matches either folder name OR article title; matching folders
// auto-expand.
const visibleRows = computed(() => {
  const q = search.value.trim().toLowerCase()
  const rows = []

  const walk = (folders, depth = 0) => {
    for (const f of folders) {
      const articles = folderArticles.value[f.id] || []
      const folderMatches = !q || f.name.toLowerCase().includes(q)
      const matchingArticles = q
        ? articles.filter((a) => a.title?.toLowerCase().includes(q))
        : articles
      const childFolderRows = [] // collected for children below

      const folderInScope = selectedFolderIds.value.has(f.id)
      const hasWhitelistedChild = articles.some((a) =>
        selectedArticleIds.value.has(a.id)
      )
      const checkboxState = folderInScope
        ? true
        : hasWhitelistedChild
        ? 'indeterminate'
        : false

      // Show the folder row when itself matches OR any descendant matches.
      // Cheap descendant check: any matching article in the lazy-loaded
      // article cache. (We don't recurse into child folders here for the
      // search check because there isn't full data to recurse over until
      // the operator expands them.)
      const folderHasMatch =
        folderMatches || matchingArticles.length > 0 || !q
      if (folderHasMatch) {
        rows.push({
          type: 'folder',
          key: `f-${f.id}`,
          id: f.id,
          title: f.name,
          folderLabel: '—',
          updated: formatUpdated(f.updated_at),
          checkboxState,
        })
      }

      // Article rows (only when expanded).
      if (expanded.value.has(f.id) || q) {
        for (const a of matchingArticles) {
          const articleInScope =
            selectedArticleIds.value.has(a.id) || selectedFolderIds.value.has(f.id)
          rows.push({
            type: 'article',
            key: `a-${a.id}`,
            id: a.id,
            folderId: f.id,
            title: a.title,
            folderLabel: f.name,
            updated: formatUpdated(a.updated_at),
            checkboxState: articleInScope,
          })
        }
      }

      // Recurse into child folders. Visible-rows ordering: a folder's
      // children appear AFTER its own articles, but that's fine for v1
      // — most KBs are 1 level deep.
      if (f.children?.length) walk(f.children, depth + 1)
    }
  }
  walk(folderTree.value)
  return rows
})

function onCheckboxChange(row, v) {
  if (!agent.value) return
  // Coerce 'indeterminate' style updates to a boolean — clicking an
  // indeterminate folder behaves like "check it" (Radix/shadcn pattern).
  const on = v === true
  const scope = agent.value.kb_scope || { folder_ids: [], article_ids: [] }
  if (row.type === 'folder') {
    const next = new Set((scope.folder_ids || []).map(Number))
    if (on) next.add(row.id)
    else next.delete(row.id)
    agent.value.kb_scope = {
      folder_ids: [...next].sort((a, b) => a - b),
      article_ids: scope.article_ids || [],
    }
  } else {
    const next = new Set((scope.article_ids || []).map(Number))
    if (on) next.add(row.id)
    else next.delete(row.id)
    agent.value.kb_scope = {
      folder_ids: scope.folder_ids || [],
      article_ids: [...next].sort((a, b) => a - b),
    }
  }
}

// Article totals + in-scope count. Counts only articles we've loaded
// (lazy fetch). totalArticles is the sum of cached folder article lists.
const totalArticles = computed(() => {
  let n = 0
  for (const k of Object.keys(folderArticles.value)) {
    n += folderArticles.value[k]?.length || 0
  }
  return n
})

const inScopeCount = computed(() => {
  // Articles whose folder is in folder_ids + articles explicitly
  // listed in article_ids, deduped.
  const seen = new Set()
  for (const k of Object.keys(folderArticles.value)) {
    const folderId = Number(k)
    const list = folderArticles.value[k] || []
    for (const a of list) {
      if (selectedFolderIds.value.has(folderId) || selectedArticleIds.value.has(a.id)) {
        seen.add(a.id)
      }
    }
  }
  return seen.size
})

function formatUpdated(iso) {
  if (!iso) return '—'
  try {
    const d = new Date(iso)
    return d.toLocaleDateString()
  } catch {
    return '—'
  }
}

// Auto-expand folders when search has results in them, so the
// matching article rows are visible without manual chevron clicks.
watch(search, async (q) => {
  if (!q.trim()) return
  // Load articles for every folder once when searching — cheap at
  // the catalog sizes we care about (< 1000 articles).
  await loadAllFolderArticles()
})

onMounted(async () => {
  try {
    const r = await api.getKnowledgeFolders()
    folders.value = r.data.data || []
    if (selectedArticleIds.value.size > 0) {
      await loadAllFolderArticles()
    }
  } catch {
    folders.value = []
  } finally {
    loading.value = false
  }
})
</script>
