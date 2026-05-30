<template>
  <slot />
</template>

<script setup>
import { computed, provide, watch } from 'vue'
import { useRoute } from 'vue-router'
import { AGENT_EDIT_KEY, useAgentEdit } from '@/composables/useAgentEdit.js'
import { AGENT_PREVIEW_KEY, useAgentPreview } from '@/composables/useAgentPreview.js'

const route = useRoute()
const agentId = computed(() => {
  const id = Number(route.params.id)
  return Number.isFinite(id) && id > 0 ? id : null
})

// Build the edit/preview state once above both the shared sidebar and the
// route content so either side can inject the same live instance.
const edit = useAgentEdit(agentId)
const preview = useAgentPreview(agentId)

provide(AGENT_EDIT_KEY, edit)
provide(AGENT_PREVIEW_KEY, preview)

watch(
  agentId,
  (next, prev) => {
    // Reset per-agent preview session state when deep-linking between
    // records, but keep the persisted rail open/closed preference.
    if (next !== prev) {
      if (next) preview.resetSession()
      edit.load()
    }
  },
  { immediate: true }
)
</script>
