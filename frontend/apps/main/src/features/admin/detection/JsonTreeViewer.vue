<template>
  <div class="space-y-1 text-sm">
    <div v-for="node in nodes" :key="node.key" class="space-y-1">
      <details v-if="node.isBranch" class="group" open>
        <summary class="cursor-pointer rounded px-2 py-1 hover:bg-muted">
          <span class="font-medium">{{ node.key }}</span>
          <span class="ml-2 text-xs text-muted-foreground">{{ node.type }}</span>
        </summary>
        <div class="ml-4 border-l border-border pl-2">
          <JsonTreeViewer :value="node.value" :base-path="node.path" @select-path="emitPath" />
        </div>
      </details>
      <button
        v-else
        type="button"
        class="flex w-full items-center justify-between gap-3 rounded px-2 py-1 text-left hover:bg-muted"
        @click="emitPath(node.path)"
      >
        <span class="font-medium">{{ node.key }}</span>
        <span class="truncate text-xs text-muted-foreground">{{ formatValue(node.value) }}</span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

defineOptions({ name: 'JsonTreeViewer' })

const props = defineProps({
  value: { type: null, required: true },
  basePath: { type: String, default: '' }
})
const emit = defineEmits(['select-path'])

const nodes = computed(() => {
  if (!props.value || typeof props.value !== 'object') return []
  if (Array.isArray(props.value)) {
    return props.value.map((value, index) => buildNode(String(index), value))
  }
  return Object.keys(props.value)
    .sort()
    .map((key) => buildNode(key, props.value[key]))
})

const buildNode = (key, value) => {
  const path = props.basePath ? `${props.basePath}.${key}` : key
  const isBranch = value !== null && typeof value === 'object'
  return {
    key,
    value,
    path,
    isBranch,
    type: Array.isArray(value) ? 'array' : 'object'
  }
}

const formatValue = (value) => {
  if (value === null || value === undefined) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

const emitPath = (path) => {
  emit('select-path', path)
}
</script>
