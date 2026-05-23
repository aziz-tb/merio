<template>
  <div class="prose prose-sm dark:prose-invert max-w-none" v-html="rendered" />
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
})

const rendered = computed(() => {
  if (!props.source) return ''
  return md.render(props.source)
})
</script>
