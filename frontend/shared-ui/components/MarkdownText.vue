<template>
  <div
    :class="[
      'prose prose-sm dark:prose-invert max-w-none',
      wrap ? 'markdown-text-wrap' : ''
    ]"
    v-html="rendered"
  />
</template>

<script setup>
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'

const md = new MarkdownIt({
  html: false,
  breaks: true,
  linkify: true,
})

const props = defineProps({
  source: { type: String, default: '' },
  wrap: { type: Boolean, default: false },
})

const rendered = computed(() => {
  if (!props.source) return ''
  return md.render(props.source)
})
</script>

<style scoped>
.markdown-text-wrap {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.markdown-text-wrap :deep(a),
.markdown-text-wrap :deep(code) {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.markdown-text-wrap :deep(pre) {
  max-width: 100%;
  overflow-x: visible;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
}
</style>
