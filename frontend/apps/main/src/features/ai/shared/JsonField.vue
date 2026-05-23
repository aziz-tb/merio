<!--
  JsonField — CodeMirror JSON editor with parseability check on blur.
  Server-side validation does the real work; this component only catches
  obviously malformed input client-side so users get fast feedback.

  Used by both CreateOrEditAgent and CreateOrEditTool for: detection,
  tools, autonomy, input_parameters, output_schema.
-->
<template>
  <div>
    <CodeEditor
      v-model="text"
      language="json"
      :height-class="heightClass"
      :title="title"
      :show-language-label="showLanguageLabel"
      :expandable="expandable"
      @blur.native.capture="validate"
    />
    <p v-if="parseError" class="text-sm text-destructive mt-1">
      {{ $t('admin.ai.invalidJson') }}: {{ parseError }}
    </p>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import CodeEditor from '@/components/editor/CodeEditor.vue'

const props = defineProps({
  // v-model gives a JSON string; parent serializes/deserializes its own object.
  modelValue: { type: String, default: '' },
  heightClass: { type: String, default: 'h-48' },
  title: { type: String, default: '' },
  showLanguageLabel: { type: Boolean, default: false },
  expandable: { type: Boolean, default: false }
})
const emit = defineEmits(['update:modelValue', 'parse-error'])

const text = ref(props.modelValue || '')
const parseError = ref('')

watch(text, (v) => emit('update:modelValue', v))

watch(
  () => props.modelValue,
  (v) => {
    if (v !== text.value) text.value = v || ''
  }
)

const validate = () => {
  if (!text.value.trim()) {
    parseError.value = ''
    emit('parse-error', '')
    return
  }
  try {
    JSON.parse(text.value)
    parseError.value = ''
    emit('parse-error', '')
  } catch (e) {
    parseError.value = e.message
    emit('parse-error', e.message)
  }
}

defineExpose({ validate })
</script>
