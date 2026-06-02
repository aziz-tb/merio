<template>
    <div :class="wrapperClasses">
        <div
            v-if="hasHeader"
            class="flex items-center justify-between px-3 py-2  bg-background shrink-0"
        >
            <div class="flex items-center gap-2">
                <span v-if="title" class="text-sm font-medium text-foreground">{{ title }}</span>
            </div>
            <button
                v-if="expandable"
                type="button"
                class="size-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                @click="isFullscreen = !isFullscreen"
                :title="isFullscreen ? 'Exit fullscreen' : 'Fullscreen'"
            >
                <X v-if="isFullscreen" class="size-4" />
                <ArrowUpRightAndArrowDownLeftFromCenter v-else class="size-4" />
            </button>
        </div>
        <div ref="codeEditor" @click="editorView?.focus()" :class="bodyClasses" />
    </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick, useTemplateRef } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { html } from '@codemirror/lang-html'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { python } from '@codemirror/lang-python'
import { oneDark } from '@codemirror/theme-one-dark'
import { useColorMode } from '@vueuse/core'
import { ArrowUpRightAndArrowDownLeftFromCenter, X } from '@qall/icons'

const props = defineProps({
    modelValue: { type: String, default: '' },
    // Supported languages: html (default), javascript, json, python, text.
    language: { type: String, default: 'html' },
    disabled: Boolean,
    readOnly: Boolean,
    // Tailwind height class. Ignored when readOnly (read-only editors size to content).
    heightClass: { type: String, default: 'h-[28rem]' },
    // Optional header — renders above the editor when any of these is set.
    title: { type: String, default: '' },
    showLanguageLabel: { type: Boolean, default: false },
    // Adds a fullscreen toggle in the header (Escape also exits).
    expandable: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue'])
const data = ref('')
let editorView = null
const codeEditor = useTemplateRef('codeEditor')
const isFullscreen = ref(false)

const hasHeader = computed(
    () => !!props.title || props.showLanguageLabel || props.expandable
)

const wrapperClasses = computed(() => {
    if (isFullscreen.value) {
        return 'fixed inset-0 z-50 bg-background border flex flex-col'
    }
    const parts = ['w-full', 'border', 'rounded-md', 'overflow-hidden']
    if (hasHeader.value) parts.push('flex', 'flex-col')
    if (!props.readOnly) parts.push(props.heightClass)
    return parts.join(' ')
})

const bodyClasses = computed(() => {
    if (isFullscreen.value) return 'flex-1 min-h-0'
    if (props.readOnly) return 'w-full'
    return hasHeader.value ? 'flex-1 min-h-0' : 'w-full h-full'
})

const langExtensionFor = (lang) => {
    switch (lang) {
        case 'javascript': return javascript()
        case 'json': return json()
        case 'python': return python()
        case 'text': return null
        case 'html':
        default: return html()
    }
}

const initCodeEditor = (body) => {
    const isDark = useColorMode().value === 'dark'
    const langExtension = langExtensionFor(props.language)
    const isEditable = !props.disabled && !props.readOnly

    // Light-mode theme mapping CodeMirror internals to design tokens.
    // Skipped in dark mode — oneDark covers that case.
    const lightTheme = EditorView.theme({
        '&': {
            backgroundColor: 'hsl(var(--background))',
            color: 'hsl(var(--foreground))',
            fontSize: '13px'
        },
        '.cm-content': {
            caretColor: 'hsl(var(--foreground))',
            padding: '12px 0'
        },
        '.cm-line': {
            padding: '0 12px'
        },
        '.cm-gutters': {
            backgroundColor: 'hsl(var(--background))',
            borderRight: '0px solid hsl(var(--border))',
            color: 'hsl(var(--muted-foreground))',
        },
        '.cm-activeLine': {
            backgroundColor: 'transparent'
        },
        '.cm-activeLineGutter': {
            backgroundColor: 'transparent',
            color: 'hsl(var(--foreground))'
        },
        '.cm-selectionBackground, ::selection': {
            backgroundColor: 'hsl(var(--accent)) !important'
        },
        '.cm-focused .cm-selectionBackground': {
            backgroundColor: 'hsl(var(--accent)) !important'
        },
        '.cm-cursor, .cm-dropCursor': {
            borderLeftColor: 'hsl(var(--foreground))'
        }
    })

    editorView = new EditorView({
        doc: body,
        extensions: [
            basicSetup,
            ...(langExtension ? [langExtension] : []),
            ...(isDark ? [oneDark] : [lightTheme]),
            EditorView.editable.of(isEditable),
            EditorView.theme({
                '&': { height: props.readOnly ? 'auto' : '100%' },
                '.cm-editor': { height: props.readOnly ? 'auto' : '100%' },
                '.cm-scroller': { overflow: 'auto' }
            }),
            EditorView.updateListener.of((update) => {
                if (!update.docChanged || props.readOnly) return
                const v = update.state.doc.toString()
                emit('update:modelValue', v)
                data.value = v
            })
        ],
        parent: codeEditor.value
    })

    if (!props.readOnly) {
        nextTick(() => editorView?.focus())
    }
}

const handleEscape = (e) => {
    if (e.key === 'Escape' && isFullscreen.value) isFullscreen.value = false
}

onMounted(() => {
    initCodeEditor(props.modelValue || '')
    document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
    document.removeEventListener('keydown', handleEscape)
})

watch(
    () => props.modelValue,
    (newVal) => {
        if (newVal !== data.value) {
            editorView?.dispatch({
                changes: { from: 0, to: editorView.state.doc.length, insert: newVal }
            })
        }
    }
)
</script>
