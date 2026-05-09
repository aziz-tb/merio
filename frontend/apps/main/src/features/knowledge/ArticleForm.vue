<!-- Page-side of the article form: just the editor, full body height.
     Title lives in the top bar (CreateArticle / EditArticle); metadata fields
     live in the secondary sidebar (KnowledgeSidebar). State is shared via the
     knowledge Pinia store. -->
<template>
  <KbArticleEditor
    ref="editorRef"
    :model-value="bodyState"
    :placeholder="t('admin.knowledge.body.placeholder')"
    class="h-full"
    @update:model-value="bodyState = $event"
  />
</template>

<script setup>
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import KbArticleEditor from './KbArticleEditor.vue'
import { getBody } from './utils/bodyConvert.js'

const { t } = useI18n()

const props = defineProps({
  // Initial body for edit mode (HTML / JSON / Markdown). Null in create mode.
  initialBody: { type: Object, default: null }
})

const editorRef = ref(null)
const bodyState = ref(
  props.initialBody || { html: '', json: { type: 'doc', content: [] }, markdown: '' }
)

watch(
  () => props.initialBody,
  (next) => {
    if (next) bodyState.value = next
  },
  { immediate: true }
)

// Read the editor content fresh on demand. Submit handlers in Create/Edit
// call this via the exposed ref to avoid trusting the debounced v-model.
const readBody = () => {
  const ed = editorRef.value?.getEditor?.()
  return ed ? getBody(ed) : bodyState.value
}

defineExpose({ readBody })
</script>
