<!--
  AgentEditHeader — top strip of the Edit Agent layout.

  Layout: [collapse  section title ..........  Save  Preview]

  - Sidebar collapse button on the far left, matching the
    ConversationList / PageHeader convention (PanelLeft icon +
    useLayoutSidebar.toggle). Toggles the global icon rail, not the
    editor's own left rail.
  - Section title derived from the active child route name
    (edit-ai-agent-{section} → admin.ai.agent.tab.{section}). Kept
    self-contained here so the router doesn't need a per-route
    meta.titleKey for this layout.
  - Save button is the sole save-state surface: disabled when not
    dirty, shows a spinner + "Saving…" label while in flight. The
    separate "Saved ✓" / "Unsaved changes" strip from the previous
    iteration has been dropped — the button itself is the indicator.
    Cmd+S / Ctrl+S still handled inside useAgentEdit (works from any
    focus).
  - Preview button: plain text, emits toggle-preview. Parent owns
    previewOpen.
-->
<template>
  <header class="flex items-center gap-3 px-4 h-14 border-b border-input shrink-0">
    <button
      type="button"
      class="cursor-pointer inline-flex items-center justify-center size-8 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground"
      @click="toggleSidebar"
    >
      <PanelLeft class="size-4" />
      <span class="sr-only">Toggle sidebar</span>
    </button>

    <h1 class="text-base font-semibold text-foreground truncate">
      {{ sectionTitle }}
    </h1>

    <div class="flex items-center gap-3 ml-auto">
      <Button
        type="button"
        :disabled="!edit?.isDirty.value || edit?.saving.value"
        @click="onSave"
      >
        <Loader2 v-if="edit?.saving.value" class="w-3.5 h-3.5 mr-1.5 animate-spin" />
        {{ edit?.saving.value ? t('globals.messages.saving') : t('globals.messages.save') }}
      </Button>

      <!-- Preview reopen button. Plain text, no icon. Hidden when
           the panel is open; the X inside the panel closes it. -->
      <button
        v-if="!previewOpen"
        type="button"
        class="text-sm text-muted-foreground hover:text-foreground transition-colors"
        @click="$emit('toggle-preview')"
      >
        {{ t('globals.terms.preview') }}
      </button>
    </div>
  </header>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Loader2, PanelLeft } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

defineProps({
  previewOpen: { type: Boolean, default: false },
})

defineEmits(['toggle-preview'])

const { t } = useI18n()
const route = useRoute()
const { toggle: toggleSidebar } = useLayoutSidebar()

const edit = inject(AGENT_EDIT_KEY)

// Section title derived from the active child route name. The sidebar
// items live at route names like `edit-ai-agent-identity`, which map
// 1:1 to the labelKey pattern `admin.ai.agent.tab.identity`. Keeping
// the derivation here (instead of pulling from sidebar items or
// route.meta) avoids touching the router and keeps the header
// self-contained.
const sectionTitle = computed(() => {
  const name = route.name?.toString() || ''
  const m = name.match(/^edit-ai-agent-(.+)$/)
  if (!m) return ''
  return t(`admin.ai.agent.tab.${m[1]}`)
})

function onSave() {
  edit?.save()
}
</script>