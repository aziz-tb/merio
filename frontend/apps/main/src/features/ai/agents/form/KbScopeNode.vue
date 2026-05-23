<template>
  <li>
    <div class="flex items-center gap-2 text-sm">
      <button
        type="button"
        class="text-muted-foreground w-4"
        :title="expanded ? $t('globals.messages.collapse') : $t('globals.messages.expand')"
        @click="toggleExpand"
      >
        {{ hasChildren || hasArticles ? (expanded ? '▾' : '▸') : '·' }}
      </button>
      <Checkbox
        :checked="folderSelected"
        @update:checked="(v) => $emit('toggle-folder', { id: folder.id, on: !!v })"
      />
      <span>{{ folder.name }}</span>
    </div>
    <ul v-if="expanded" class="pl-6 space-y-1 mt-1">
      <li v-for="a in articles" :key="`a-${a.id}`" class="flex items-center gap-2 text-xs">
        <span class="text-muted-foreground w-4">·</span>
        <Checkbox
          :checked="articleSelectedIds.has(a.id)"
          @update:checked="(v) => $emit('toggle-article', { id: a.id, on: !!v })"
        />
        <span class="font-mono">#{{ a.id }}</span>
        <span>{{ a.title }}</span>
      </li>
      <KBScopeNode
        v-for="child in folder.children"
        :key="`f-${child.id}`"
        :folder="child"
        :folder-articles="folderArticles"
        :selected-folder-ids="selectedFolderIds"
        :selected-article-ids="selectedArticleIds"
        :load-articles="loadArticles"
        @toggle-folder="$emit('toggle-folder', $event)"
        @toggle-article="$emit('toggle-article', $event)"
      />
    </ul>
  </li>
</template>

<script setup>
// Recursive node for KBScopePicker. Folder checkbox + lazy-loaded
// article list. Children render via self-recursion (Vue allows self
// reference when the component has a name; <script setup> uses the
// filename, so this works without an explicit `name:` option).
import { computed, ref } from 'vue'
import { Checkbox } from '@shared-ui/components/ui/checkbox'

const props = defineProps({
  folder: { type: Object, required: true },
  folderArticles: { type: Object, required: true },
  selectedFolderIds: { type: Object, required: true }, // Set<number>
  selectedArticleIds: { type: Object, required: true }, // Set<number>
  loadArticles: { type: Function, required: true }
})

defineEmits(['toggle-folder', 'toggle-article'])

const expanded = ref(false)
const hasChildren = computed(() => (props.folder.children || []).length > 0)
const articles = computed(() => props.folderArticles[props.folder.id] || [])
const hasArticles = computed(() => articles.value.length > 0)
const folderSelected = computed(() => props.selectedFolderIds.has(props.folder.id))
const articleSelectedIds = computed(() => props.selectedArticleIds)

const toggleExpand = async () => {
  expanded.value = !expanded.value
  if (expanded.value && !props.folderArticles[props.folder.id]) {
    await props.loadArticles(props.folder.id)
  }
}
</script>
