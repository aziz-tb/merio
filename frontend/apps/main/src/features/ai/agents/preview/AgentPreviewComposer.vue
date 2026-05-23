<!--
  AgentPreviewComposer — compose box pinned at the bottom of the
  Preview rail.

  Layout:
    ┌───────────────────────────────┐
    │ [textarea]                    │
    │                               │
    │ ☐ Safe mode          [▶ Run]  │
    └───────────────────────────────┘

  A single inline action row: safe-mode checkbox on the left, Run on
  the right. Same radius as the surrounding input box (no rounded-full
  on Run). The decorative paperclip from the previous iteration has
  been removed — there was no handler behind it.

  Cmd/Ctrl+Enter triggers Run from the textarea.
-->
<template>
  <div class="px-4 py-3 shrink-0 bg-background">
    <div
      class="border border-input rounded-lg overflow-hidden bg-background focus-within:ring-1 focus-within:ring-ring"
    >
      <textarea
        v-model="preview.composeBody.value"
        :placeholder="t('admin.ai.agent.preview.composePlaceholder')"
        class="w-full min-h-[80px] max-h-[300px] bg-transparent px-3 py-2.5 text-sm placeholder:text-muted-foreground resize-none focus:outline-none"
        @keydown.meta.enter.prevent="onRun"
        @keydown.ctrl.enter.prevent="onRun"
      />

      <div class="flex items-center justify-between gap-3 px-3 pb-2.5">
        <label class="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer">
          <Checkbox
            :checked="preview.safeMode.value"
            @update:checked="(v) => (preview.safeMode.value = v === true)"
          />
          {{ t('admin.ai.agent.preview.safeMode') }}
        </label>

        <Button
          type="button"
          size="sm"
          :disabled="runDisabled"
          @click="onRun"
        >
          <Loader2 v-if="preview.running.value" class="w-3.5 h-3.5 mr-1.5 animate-spin" />
          <Send v-else class="w-3.5 h-3.5 mr-1.5" />
          {{ t('globals.messages.run') }}
        </Button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, Send } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { AGENT_PREVIEW_KEY } from '@/composables/useAgentPreview.js'

const { t } = useI18n()
const preview = inject(AGENT_PREVIEW_KEY)

const runDisabled = computed(
  () => preview.running.value || !preview.composeBody.value.trim()
)

function onRun() {
  if (runDisabled.value) return
  preview.run()
}
</script>
