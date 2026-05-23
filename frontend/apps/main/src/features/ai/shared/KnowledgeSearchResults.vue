<template>
  <div class="space-y-3">
    <div
      v-for="result in results"
      :key="result.article_id"
      class="rounded-md border border-input bg-background p-3 space-y-2"
    >
      <RouterLink
        :to="{ name: 'edit-article', params: { id: result.article_id } }"
        target="_blank"
        class="flex items-center gap-2 text-sm font-medium text-foreground hover:underline"
      >
        <BookOpen class="h-4 w-4 shrink-0 text-muted-foreground" />
        <span>{{ result.title }}</span>
      </RouterLink>

      <p
        v-if="sectionPath(result)"
        class="text-xs text-muted-foreground"
      >
        {{ sectionPath(result) }}
      </p>

      <MarkdownText :source="chunkContent(result)" />
    </div>
  </div>
</template>

<script setup>
import { BookOpen } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import MarkdownText from '@shared-ui/components/MarkdownText.vue'

defineProps({
  results: { type: Array, default: () => [] },
})

const sectionPath = (result) => {
  const raw = result.matched_chunks?.[0]?.source_path
  if (!raw) return ''
  return raw
    .split(/\s+>\s+/)
    .map((s) => s.replace(/^#{1,6}\s*/, '').trim())
    .filter(Boolean)
    .join(' › ')
}

const chunkContent = (result) => {
  return result.matched_chunks?.[0]?.chunk_text || result.body_excerpt || ''
}
</script>
