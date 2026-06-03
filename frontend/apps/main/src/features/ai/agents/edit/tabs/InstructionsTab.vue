<!--
  AgentTabInstructions — the dominant editing surface.

  Layout (top→bottom):
    1. Reasoning model row: PresetSelect + muted helper showing the
       resolved model (when known).
    2. CodeMirror prompt editor (reuses CodeEditor.vue), tall.
    3. Footer strip: char + estimated token count (left) + template
       variable hints (right).

  Reads/writes the live agent record via inject. agent.prompt for
  the editor body, agent.preset_overrides.reason for the reasoning
  preset (matches the Identity-tab control — both surfaces edit the
  same field; whichever changes last wins, which is the user's intent).

  Token estimate: chars/4. Matches the F12 chunker's heuristic and is
  good enough for "is the prompt huge?" feedback.
-->
<template>
  <div class="min-h-full">
    <div v-if="!agent" class="px-6 py-6 text-sm text-muted-foreground">…</div>

    <template v-else>
      <!-- Reasoning model row -->
      <div class="px-6 pt-6 pb-3 space-y-2">
        <div class="flex items-center justify-between gap-4">
          <Label class="text-sm font-medium text-foreground">
            {{ t('admin.ai.agent.instructions.reasoningModel') }}
          </Label>
        </div>
        <div class="max-w-md">
          <PresetSelect :model-value="reasonPreset" @update:modelValue="onPresetChange" />
        </div>
        <p class="text-xs text-muted-foreground">
          {{ t('admin.ai.agent.presetOverridesHelp') }}
        </p>

        <div class="flex items-center justify-between gap-4 pt-2 max-w-md">
          <div class="space-y-1">
            <Label class="text-sm font-medium text-foreground">
              {{ t('admin.ai.agent.instructions.extendedThinking') }}
            </Label>
            <p class="text-xs text-muted-foreground">
              {{ t('admin.ai.agent.instructions.extendedThinkingHelp') }}
            </p>
          </div>
          <Switch
            :checked="agent.extended_thinking_enabled !== false"
            @update:checked="(v) => (agent.extended_thinking_enabled = v)"
          />
        </div>
      </div>

      <!-- CodeMirror prompt — grows with the prompt; the tab scrolls when needed. -->
      <div class="px-6 pb-4">
        <Label class="text-sm font-medium text-foreground mb-1.5 block">
          {{ t('globals.terms.prompt') }}
        </Label>
        <CodeEditor
          :model-value="agent.prompt || ''"
          @update:model-value="(v) => (agent.prompt = v)"
          language="text"
          min-height-class="min-h-[32rem]"
          line-wrapping
          auto-height
        />
        <p class="mt-2 text-xs text-muted-foreground">
          {{ t('admin.ai.agent.instructions.platformFinalizeHelp') }}
        </p>
      </div>

      <!-- Footer strip -->
      <div class="px-6 py-3 border-t border-input flex items-center justify-between gap-4 text-xs text-muted-foreground">
        <span>
          {{ promptChars.toLocaleString() }} {{ t('globals.terms.characters') }}
          <span class="mx-1">·</span>
          ~{{ promptTokens.toLocaleString() }} {{ t('globals.terms.tokens') }}
        </span>
        <span class="truncate" :title="templateVarsTooltip">
          {{ t('admin.ai.agent.instructions.variables') }}:
          <code class="font-mono">{{ '{current_date}' }}</code>
          <code class="ml-1.5 font-mono">{{ '{current_time}' }}</code>
          <code class="ml-1.5 font-mono">{{ '{contact_name}' }}</code>
        </span>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import CodeEditor from '@/components/editor/CodeEditor.vue'
import PresetSelect from '@/features/ai/shared/PresetSelect.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)

const promptChars = computed(() => (agent.value?.prompt ?? '').length)
// Token estimate matches F12 chunking.EstimateTokens (chars/4).
const promptTokens = computed(() => Math.ceil(promptChars.value / 4))

// Same preset_overrides.reason model as Identity tab. PresetSelect's
// '__default__' sentinel translates to deleting the key so the runner
// falls back to the platform purpose mapping.
const reasonPreset = computed(
  () => (agent.value?.preset_overrides && agent.value.preset_overrides.reason) || ''
)
function onPresetChange(slug) {
  if (!agent.value) return
  const next = { ...(agent.value.preset_overrides || {}) }
  if (!slug || slug === '__default__') delete next.reason
  else next.reason = slug
  agent.value.preset_overrides = next
}

const templateVarsTooltip = computed(
  () =>
    t('admin.ai.agent.instructions.variablesHelp')
)
</script>
