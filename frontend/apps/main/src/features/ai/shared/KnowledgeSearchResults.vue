<template>
  <div class="min-w-0 space-y-3">
    <div
      v-for="result in results"
      :key="result.article_id"
      class="min-w-0 space-y-2 p-1 pb-2"
    >
      <RouterLink
        :to="{ name: 'edit-article', params: { id: result.article_id } }"
        target="_blank"
        rel="noopener noreferrer"
        class="flex min-w-0 items-start gap-2 text-sm font-medium text-foreground hover:underline"
      >
        <BookOpen class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <span class="min-w-0 break-words">{{ result.title }}</span>
      </RouterLink>

      <div v-if="result.matched_chunks?.length" class="min-w-0 divide-y divide-border rounded-md border border-input">
        <Collapsible
          v-for="(chunk, index) in result.matched_chunks"
          :key="`${result.article_id}-${chunk.chunk_index ?? index}`"
          :default-open="index === 0"
          v-slot="{ open }"
        >
          <CollapsibleTrigger class="flex w-full items-start justify-between gap-2 px-3 py-2 text-left text-xs transition-colors hover:bg-muted/50">
            <span class="min-w-0">
              <span class="block break-words font-medium text-foreground">
                {{ chunkTitle(chunk, index) }}
              </span>
              <span v-if="sectionPath(chunk)" class="block break-words text-muted-foreground">
                {{ sectionPath(chunk) }}
              </span>
            </span>
            <ChevronDown
              class="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform"
              :class="{ 'rotate-180': open }"
            />
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div class="min-w-0 px-3 pb-3">
              <MarkdownText :source="chunk.chunk_text || ''" wrap />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>

      <MarkdownText v-else :source="result.body_excerpt || ''" wrap />
    </div>
  </div>
</template>

<script setup>
import { BookOpen, ChevronDown } from '@qall/icons'
import { RouterLink } from 'vue-router'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@shared-ui/components/ui/collapsible'
import MarkdownText from '@shared-ui/components/MarkdownText.vue'

defineProps({
  results: { type: Array, default: () => [] },
})

const chunkTitle = (chunk, index) => {
  const raw = chunk?.source_path
  if (!raw) return `Chunk ${index + 1}`
  const parts = raw.split(/\s+>\s+/)
  const title = parts[parts.length - 1]?.replace(/^#{1,6}\s*/, '').trim()
  return title || `Chunk ${index + 1}`
}

const sectionPath = (chunk) => {
  const raw = chunk?.source_path
  if (!raw) return ''
  return raw
    .split(/\s+>\s+/)
    .slice(0, -1)
    .map((s) => s.replace(/^#{1,6}\s*/, '').trim())
    .filter(Boolean)
    .join(' › ')
}
</script>
