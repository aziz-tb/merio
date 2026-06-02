<template>
  <div class="space-y-4">
    <ModeSelector v-model="detection.mode" />

    <AIPromptEditor
      v-if="showAI"
      v-model="detection.ai.prompt"
    />

    <ConditionTree
      v-if="showRules"
      v-model="detection.rules"
      :available-sources="availableSources"
    />
  </div>
</template>

<script setup>
import { computed, watch } from 'vue'
import ModeSelector from './ModeSelector.vue'
import AIPromptEditor from './AIPromptEditor.vue'
import ConditionTree from './ConditionTree.vue'

const detection = defineModel({ required: true })
defineProps({
  availableSources: {
    type: Array,
    default: () => ['ticket', 'customer', 'contract', 'tags']
  }
})

const showRules = computed(() => !['ai_only', 'manual_only'].includes(detection.value.mode))
const showAI = computed(() => !['rules_only', 'manual_only'].includes(detection.value.mode))

watch(
  detection,
  () => {
    if (!detection.value.mode) {
      detection.value.mode = 'rules_only'
    }
    if (!detection.value.rules) {
      detection.value.rules = { op: 'AND', children: [] }
    }
    if (!detection.value.ai) {
      detection.value.ai = { prompt: '' }
    }
  },
  { immediate: true, deep: true }
)
</script>
