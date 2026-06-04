<template>
  <div class="space-y-4">
    <div>
      <h4 class="text-sm font-medium mb-2">
        {{ $t('admin.ai.agent.toolsPicker.internalHeading') }}
      </h4>
      <p class="text-xs text-muted-foreground mb-2">
        {{ $t('admin.ai.agent.toolsPicker.internalHelp') }}
      </p>
      <div v-if="internalCaps.length === 0" class="text-xs text-muted-foreground">
        {{ $t('admin.ai.agent.toolsPicker.internalEmpty') }}
      </div>
      <ul class="space-y-1">
        <li
          v-for="c in internalCaps"
          :key="`int-${c.id}`"
          class="flex items-start gap-2 text-sm"
        >
          <Checkbox
            :checked="isInternalSelected(c.id)"
            @update:checked="(v) => toggleInternal(c.id, v)"
          />
          <div class="flex-1">
            <div class="font-mono text-xs">{{ c.id }}</div>
            <div class="text-xs text-muted-foreground">{{ c.name }}</div>
          </div>
        </li>
      </ul>
    </div>

    <div>
      <h4 class="text-sm font-medium mb-2">
        {{ $t('admin.ai.agent.toolsPicker.customHeading') }}
      </h4>
      <p class="text-xs text-muted-foreground mb-2">
        {{ $t('admin.ai.agent.toolsPicker.customHelp') }}
      </p>
      <div v-if="customTools.length === 0" class="text-xs text-muted-foreground">
        {{ $t('admin.ai.agent.toolsPicker.customEmpty') }}
      </div>
      <ul class="space-y-1">
        <li
          v-for="t in customTools"
          :key="`cust-${t.id}`"
          class="flex items-start gap-2 text-sm"
        >
          <Checkbox
            :checked="isCustomSelected(t.id)"
            @update:checked="(v) => toggleCustom(t.id, v)"
          />
          <div class="flex-1">
            <div class="font-mono text-xs">{{ t.slug }}</div>
            <div class="text-xs text-muted-foreground">{{ t.name }}</div>
          </div>
          <Badge v-if="!t.enabled" variant="secondary">
            {{ $t('globals.terms.disabled') }}
          </Badge>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup>
// AgentToolsPicker — checkbox list of every available tool, grouped by
// kind. Internal capabilities come from /api/v1/ai/internal-capabilities;
// custom tools from /api/v1/ai/tools.
//
// v-model carries an array of ToolRef objects in the F5-unified shape:
//   { kind: "internal" | "custom", id: <string | number> }
// config_override is preserved on already-selected refs; new selections
// don't add it (admins use the existing JSON editor for that).
import { ref, computed, onMounted } from 'vue'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Badge } from '@shared-ui/components/ui/badge'
import api from '@/api'

const props = defineProps({
  modelValue: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue'])

const internalCaps = ref([])
const customTools = ref([])
const HIDDEN_INTERNAL_TOOL_IDS = new Set(['kb_search'])

const internalIndex = computed(() => {
  const m = new Map()
  for (const r of props.modelValue) {
    if (r && r.kind === 'internal') m.set(String(r.id), r)
  }
  return m
})
const customIndex = computed(() => {
  const m = new Map()
  for (const r of props.modelValue) {
    if (r && r.kind === 'custom') m.set(Number(r.id), r)
  }
  return m
})

const isInternalSelected = (id) => internalIndex.value.has(String(id))
const isCustomSelected = (id) => customIndex.value.has(Number(id))

const emitChange = (next) => emit('update:modelValue', next)

const toggleInternal = (id, on) => {
  const next = props.modelValue.filter(
    (r) => !(r.kind === 'internal' && String(r.id) === String(id))
  )
  if (on) next.push({ kind: 'internal', id: String(id) })
  emitChange(next)
}
const toggleCustom = (id, on) => {
  const next = props.modelValue.filter(
    (r) => !(r.kind === 'custom' && Number(r.id) === Number(id))
  )
  if (on) next.push({ kind: 'custom', id: Number(id) })
  emitChange(next)
}

onMounted(async () => {
  try {
    const r = await api.getAIInternalCapabilities()
    internalCaps.value = (r.data.data || []).filter((c) => !HIDDEN_INTERNAL_TOOL_IDS.has(String(c.id)))
  } catch {
    internalCaps.value = []
  }
  try {
    const r = await api.getAITools({ page_size: 200 })
    const data = r.data.data
    customTools.value = Array.isArray(data) ? data : data?.results || []
  } catch {
    customTools.value = []
  }
})
</script>
