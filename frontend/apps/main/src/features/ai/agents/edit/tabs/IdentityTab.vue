<!--
  AgentTabIdentity — basic facts for the agent record.

  Single-column form, max-width ~560px. Reads/writes the live agent
  record from useAgentEdit via inject. All fields mutate the agent
  ref directly; dirty tracking + Save are owned by the layout.

  Fields:
    - Name
    - Slug (editable during create, read-only after save)
    - Description
    - Requires verified customer (maps to min_verification 2/0)
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

      <!-- Slug -->
      <div class="space-y-1.5">
        <Label :class="!isCreateMode ? 'text-muted-foreground' : ''">
          {{ t('globals.terms.slug') }}
        </Label>
        <Input
          :model-value="agent.slug"
          type="text"
          :readonly="!isCreateMode"
          :tabindex="isCreateMode ? undefined : -1"
          :class="!isCreateMode ? 'cursor-not-allowed bg-muted/40' : ''"
          @update:model-value="onSlugInput"
          @keydown.enter.prevent
        />
        <p v-if="!isCreateMode" class="text-xs text-muted-foreground">
          {{ t('admin.ai.slugPermanent') }}
        </p>
      </div>

      <!-- Description -->
      <div class="space-y-1.5">
        <Label>{{ t('globals.terms.description') }}</Label>
        <Input v-model="agent.description" type="text" @keydown.enter.prevent />
      </div>

      <!-- Verification gate -->
      <div class="space-y-1.5">
        <div class="flex items-center justify-between gap-3">
          <Label>{{ t('admin.ai.agent.requiresVerifiedCustomer') }}</Label>
          <Switch
            :checked="(agent.min_verification ?? 0) >= 2"
            @update:checked="(v) => (agent.min_verification = v ? 2 : 0)"
          />
        </div>
        <p class="text-xs text-muted-foreground">
          {{ t('admin.ai.agent.requiresVerifiedCustomerHelp') }}
        </p>
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
import { computed, inject, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { slugify } from '@/utils/slugify.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)
const isCreateMode = computed(() => edit?.isCreateMode.value ?? false)
const slugDirty = ref(false)

watch(
  () => agent.value?.name,
  (newName) => {
    if (!isCreateMode.value || slugDirty.value || !agent.value) return
    agent.value.slug = slugify(newName || '')
  }
)

watch(isCreateMode, (createMode) => {
  if (!createMode) slugDirty.value = false
})

function onSlugInput(value) {
  if (!isCreateMode.value || !agent.value) return
  slugDirty.value = true
  agent.value.slug = value
}
</script>
