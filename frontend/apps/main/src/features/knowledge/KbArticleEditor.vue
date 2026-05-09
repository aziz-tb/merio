<template>
  <div class="flex flex-col h-full">
    <div v-if="editor" class="flex items-center gap-1 px-2 py-1 border-y border-input bg-muted/40 shrink-0">
      <!-- 1. History -->
      <button type="button" class="kb-toolbar-btn" :title="t('admin.knowledge.toolbar.undo')" @click="editor.chain().focus().undo().run()">
        <Undo2 class="size-4" />
      </button>
      <button type="button" class="kb-toolbar-btn" :title="t('admin.knowledge.toolbar.redo')" @click="editor.chain().focus().redo().run()">
        <Redo2 class="size-4" />
      </button>
      <span class="kb-toolbar-sep" />

      <!-- 2. Heading -->
      <select
        class="h-7 text-xs rounded border border-input bg-transparent px-1"
        :value="headingValue"
        @change="onHeadingChange"
      >
        <option value="paragraph">{{ t('admin.knowledge.toolbar.paragraph') }}</option>
        <option value="h1">H1</option>
        <option value="h2">H2</option>
        <option value="h3">H3</option>
      </select>
      <span class="kb-toolbar-sep" />

      <!-- 3. Inline -->
      <button type="button" :class="btnClass(editor.isActive('bold'))" :title="t('admin.knowledge.toolbar.bold')" @click="editor.chain().focus().toggleBold().run()">
        <Bold class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('italic'))" :title="t('admin.knowledge.toolbar.italic')" @click="editor.chain().focus().toggleItalic().run()">
        <Italic class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('underline'))" :title="t('admin.knowledge.toolbar.underline')" @click="editor.chain().focus().toggleUnderline().run()">
        <UnderlineIcon class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('strike'))" :title="t('admin.knowledge.toolbar.strike')" @click="editor.chain().focus().toggleStrike().run()">
        <Strikethrough class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('code'))" :title="t('admin.knowledge.toolbar.code')" @click="editor.chain().focus().toggleCode().run()">
        <Code class="size-4" />
      </button>
      <span class="kb-toolbar-sep" />

      <!-- 4. List -->
      <button type="button" :class="btnClass(editor.isActive('bulletList'))" :title="t('admin.knowledge.toolbar.bulletList')" @click="editor.chain().focus().toggleBulletList().run()">
        <List class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('orderedList'))" :title="t('admin.knowledge.toolbar.orderedList')" @click="editor.chain().focus().toggleOrderedList().run()">
        <ListOrdered class="size-4" />
      </button>
      <span class="kb-toolbar-sep" />

      <!-- 5. Block -->
      <button type="button" :class="btnClass(editor.isActive('blockquote'))" :title="t('admin.knowledge.toolbar.blockquote')" @click="editor.chain().focus().toggleBlockquote().run()">
        <Quote class="size-4" />
      </button>
      <button type="button" :class="btnClass(editor.isActive('codeBlock'))" :title="t('admin.knowledge.toolbar.codeBlock')" @click="editor.chain().focus().toggleCodeBlock().run()">
        <SquareCode class="size-4" />
      </button>
      <span class="kb-toolbar-sep" />

      <!-- 6. Link -->
      <button type="button" :class="btnClass(editor.isActive('link'))" :title="t('admin.knowledge.toolbar.link')" @click="onLink">
        <LinkIcon class="size-4" />
      </button>
      <span class="kb-toolbar-sep" />

      <!-- 7. Align (popover replaces the wrap-prone dropdown) -->
      <Popover>
        <PopoverTrigger as-child>
          <button type="button" class="kb-toolbar-btn" :title="t(`admin.knowledge.toolbar.align${alignTitleSuffix}`)">
            <component :is="alignIcon" class="size-4" />
          </button>
        </PopoverTrigger>
        <PopoverContent class="w-auto p-1 flex gap-0.5" align="start">
          <button type="button" :class="alignBtnClass('left')" :title="t('admin.knowledge.toolbar.alignLeft')" @click="setAlign('left')">
            <AlignLeft class="size-4" />
          </button>
          <button type="button" :class="alignBtnClass('center')" :title="t('admin.knowledge.toolbar.alignCenter')" @click="setAlign('center')">
            <AlignCenter class="size-4" />
          </button>
          <button type="button" :class="alignBtnClass('right')" :title="t('admin.knowledge.toolbar.alignRight')" @click="setAlign('right')">
            <AlignRight class="size-4" />
          </button>
          <button type="button" :class="alignBtnClass('justify')" :title="t('admin.knowledge.toolbar.alignJustify')" @click="setAlign('justify')">
            <AlignJustify class="size-4" />
          </button>
        </PopoverContent>
      </Popover>
      <span class="kb-toolbar-sep" />

      <!-- Insert -->
      <button type="button" class="kb-toolbar-btn" :title="t('admin.knowledge.toolbar.horizontalRule')" @click="editor.chain().focus().setHorizontalRule().run()">
        <Minus class="size-4" />
      </button>
    </div>
    <EditorContent :editor="editor" class="kb-editor-content flex-1 min-h-0 overflow-auto" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import Underline from '@tiptap/extension-underline'
import TextAlign from '@tiptap/extension-text-align'
import { Markdown } from 'tiptap-markdown'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@shared-ui/components/ui/popover/index.js'
import {
  AlignCenter,
  AlignJustify,
  AlignLeft,
  AlignRight,
  Bold,
  Code,
  Italic,
  Link as LinkIcon,
  List,
  ListOrdered,
  Minus,
  Quote,
  Redo2,
  SquareCode,
  Strikethrough,
  Underline as UnderlineIcon,
  Undo2
} from 'lucide-vue-next'
import { getBody } from './utils/bodyConvert.js'

const { t } = useI18n()

const props = defineProps({
  modelValue: {
    type: Object,
    default: () => ({ html: '', json: { type: 'doc', content: [] }, markdown: '' })
  },
  placeholder: { type: String, default: '' }
})
const emit = defineEmits(['update:modelValue'])

const editor = ref(null)
const headingValue = ref('paragraph')
const alignValue = ref('left')

let debounceTimer = null
const DEBOUNCE_MS = 200

const recomputeToolbarState = () => {
  if (!editor.value) return
  if (editor.value.isActive('heading', { level: 1 })) headingValue.value = 'h1'
  else if (editor.value.isActive('heading', { level: 2 })) headingValue.value = 'h2'
  else if (editor.value.isActive('heading', { level: 3 })) headingValue.value = 'h3'
  else headingValue.value = 'paragraph'

  if (editor.value.isActive({ textAlign: 'center' })) alignValue.value = 'center'
  else if (editor.value.isActive({ textAlign: 'right' })) alignValue.value = 'right'
  else if (editor.value.isActive({ textAlign: 'justify' })) alignValue.value = 'justify'
  else alignValue.value = 'left'
}

onMounted(() => {
  editor.value = new Editor({
    extensions: [
      StarterKit,
      Link.configure({ openOnClick: false }),
      Placeholder.configure({ placeholder: props.placeholder }),
      Underline,
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Markdown
    ],
    content:
      props.modelValue?.json && Object.keys(props.modelValue.json).length
        ? props.modelValue.json
        : props.modelValue?.html || '',
    onUpdate: () => {
      // Debounced v-model emission. Submit handlers MUST recompute via
      // bodyConvert.getBody to capture the final keystroke — see ArticleForm.
      if (debounceTimer) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => {
        emit('update:modelValue', getBody(editor.value))
      }, DEBOUNCE_MS)
      recomputeToolbarState()
    },
    onSelectionUpdate: recomputeToolbarState
  })
  recomputeToolbarState()
})

onBeforeUnmount(() => {
  if (debounceTimer) clearTimeout(debounceTimer)
  editor.value?.destroy()
})

watch(
  () => props.modelValue?.json,
  (next) => {
    if (!editor.value || !next) return
    const current = editor.value.getJSON()
    if (JSON.stringify(current) === JSON.stringify(next)) return
    editor.value.commands.setContent(next, false)
    recomputeToolbarState()
  }
)

const btnClass = (active) => ['kb-toolbar-btn', active ? 'bg-muted text-foreground' : '']

const alignIcon = computed(() => {
  switch (alignValue.value) {
    case 'center':
      return AlignCenter
    case 'right':
      return AlignRight
    case 'justify':
      return AlignJustify
    default:
      return AlignLeft
  }
})

const alignTitleSuffix = computed(() => {
  switch (alignValue.value) {
    case 'center':
      return 'Center'
    case 'right':
      return 'Right'
    case 'justify':
      return 'Justify'
    default:
      return 'Left'
  }
})

const alignBtnClass = (which) =>
  ['kb-toolbar-btn', alignValue.value === which ? 'bg-muted text-foreground' : '']

const setAlign = (v) => {
  editor.value.chain().focus().setTextAlign(v).run()
}

const onHeadingChange = (e) => {
  const v = e.target.value
  if (v === 'paragraph') editor.value.chain().focus().setParagraph().run()
  else editor.value.chain().focus().toggleHeading({ level: Number(v[1]) }).run()
}

const onLink = () => {
  const prev = editor.value.getAttributes('link').href || ''
  // eslint-disable-next-line no-alert
  const url = window.prompt(t('admin.knowledge.toolbar.linkPrompt'), prev)
  if (url === null) return
  if (url === '') {
    editor.value.chain().focus().extendMarkRange('link').unsetLink().run()
    return
  }
  editor.value.chain().focus().extendMarkRange('link').setLink({ href: url, target: '_blank' }).run()
}

defineExpose({ getEditor: () => editor.value })
</script>

<style scoped>
.kb-toolbar-btn {
  @apply inline-flex items-center justify-center size-7 rounded text-muted-foreground hover:text-foreground hover:bg-muted;
}
.kb-toolbar-sep {
  @apply inline-block w-px h-5 bg-border mx-1;
}
.kb-editor-content :deep(.ProseMirror) {
  @apply min-h-full px-12 py-6 outline-none prose prose-sm max-w-none;
}
.kb-editor-content :deep(.ProseMirror p.is-editor-empty:first-child::before) {
  @apply text-muted-foreground;
  content: attr(data-placeholder);
  float: left;
  height: 0;
  pointer-events: none;
}
</style>
