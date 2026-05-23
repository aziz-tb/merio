<!--
  AgentTabIdentity — basic facts for the agent record.

  Single-column form, max-width ~560px. Reads/writes the live agent
  record from useAgentEdit via inject. All fields mutate the agent
  ref directly; dirty tracking + Save are owned by the layout.

  Fields:
    - Name
    - Slug (read-only, locked at create-time per backend rule)
    - Description
    - Minimum trust level (legitimation; renamed for clarity)
    - Enabled (switch)

  Reasoning preset lives on the Instructions tab — it's part of the
  prompt/model surface, not identity. It was previously duplicated
  here; the duplicate has been removed.

  No vee-validate here: the underlying model (agent record) is owned
  by useAgentEdit, not a form. Per-field validation lives server-side
  on the PUT and surfaces via the toast.
-->
<template>
  <div class="max-w-xl px-6 py-6 space-y-5">
    <div v-if="!agent" class="text-sm text-muted-foreground">…</div>

    <template v-else>
      <!-- Name -->
      <div class="space-y-1.5">
        <Label>{{ t('globals.terms.name') }}</Label>
        <Input v-model="agent.name" type="text" @keydown.enter.prevent />
      </div>

      <!-- Slug — read-only -->
      <div class="space-y-1.5">
        <Label class="text-muted-foreground">{{ t('globals.terms.slug') }}</Label>
        <Input
          :model-value="agent.slug"
          type="text"
          readonly
          tabindex="-1"
          class="cursor-not-allowed bg-muted/40"
        />
        <p class="text-xs text-muted-foreground">{{ t('admin.ai.slugPermanent') }}</p>
      </div>

      <!-- Description -->
      <div class="space-y-1.5">
        <Label>{{ t('globals.terms.description') }}</Label>
        <Input v-model="agent.description" type="text" @keydown.enter.prevent />
      </div>

      <!-- Minimum trust level -->
      <div class="space-y-1.5">
        <Label>{{ t('admin.ai.agent.minLegitimation') }}</Label>
        <Input
          type="number"
          step="1"
          min="0"
          :model-value="agent.min_legitimation ?? 0"
          @input="(e) => (agent.min_legitimation = e.target.value === '' ? 0 : Number(e.target.value))"
        />
        <p class="text-xs text-muted-foreground">{{ t('admin.ai.agent.minLegitimationHelp') }}</p>
      </div>

      <!-- Enabled -->
      <div class="flex items-center justify-between gap-3">
        <Label class="text-foreground">{{ t('globals.terms.enabled') }}</Label>
        <Switch :checked="agent.enabled" @update:checked="(v) => (agent.enabled = v)" />
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)
</script>