<!--
  AgentTabTools — rich-rows surface for built-in capabilities + custom
  tools. Single uniform list (no grouping). Each row toggles whether
  the tool ref lands in agent.tools.

  ToolRef shape (mirrors AgentToolsPicker for backend compatibility):
    { kind: 'internal' | 'custom', id: string | number, config_override? }

  Reads/writes the live agent record via inject.

-->
<template>
  <div class="mx-auto px-6 py-6 max-w-5xl">
    <div class="flex items-center justify-between gap-4 mb-3">
      <h2 class="text-lg font-semibold">{{ t('admin.ai.agent.tab.tools') }}</h2>
      <span class="text-xs text-muted-foreground">
        {{ enabledCount }} {{ t('globals.terms.of') }} {{ rows.length }}
        {{ t('globals.terms.enabled') }}
      </span>
    </div>
    <p class="text-sm text-muted-foreground mb-4">
      {{ t('admin.ai.agent.tools.help') }}
    </p>

    <div class="mb-3">
      <Input
        v-model="search"
        type="text"
        :placeholder="t('globals.messages.search')"
        @keydown.enter.prevent
      />
    </div>

    <div v-if="loading" class="text-sm text-muted-foreground">
      {{ t('globals.messages.loading') }}
    </div>
    <div v-else-if="filteredRows.length === 0" class="text-sm text-muted-foreground py-6 text-center border border-dashed border-input rounded-md">
      {{ t('admin.ai.agent.tools.empty') }}
    </div>
    <ul v-else class="border border-input rounded-md divide-y divide-input">
      <li
        v-for="row in filteredRows"
        :key="row.refKey"
        class="flex items-start gap-3 px-4 py-3"
      >
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-foreground truncate">{{ row.name }}</span>
            <span
              v-if="row.accessType"
              class="text-[10px] uppercase tracking-wide bg-muted text-muted-foreground rounded px-1.5 py-0.5"
            >
              {{ row.accessType }}
            </span>
          </div>
          <div class="mt-0.5 text-xs text-muted-foreground line-clamp-2">
            {{ row.description }}
            <template v-if="row.metadata">
              <span class="mx-1">·</span>
              <span class="font-mono">{{ row.metadata }}</span>
            </template>
          </div>
        </div>
        <Switch
          :checked="isSelected(row)"
          @update:checked="(v) => toggle(row, v)"
        />
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Switch } from '@shared-ui/components/ui/switch'
import api from '@/api'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)

const search = ref('')
const loading = ref(true)
const internalCaps = ref([])
const customTools = ref([])
const HIDDEN_INTERNAL_TOOL_IDS = new Set(['kb_search'])

onMounted(async () => {
  loading.value = true
  try {
    const r = await api.getAIInternalCapabilities()
    internalCaps.value = r.data.data || []
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
  loading.value = false
})

// Unified row shape for rendering. refKey gives Vue a stable key
// across internal vs custom tools without colliding.
const rows = computed(() => {
  const out = []
  for (const c of internalCaps.value) {
    if (HIDDEN_INTERNAL_TOOL_IDS.has(String(c.id))) continue
    out.push({
      kind: 'internal',
      id: String(c.id),
      refKey: `int:${c.id}`,
      name: c.name || c.id,
      description: c.description || '',
      metadata: c.id, // capability slug as the secondary identifier
      accessType: c.access_type || '',
    })
  }
  for (const tool of customTools.value) {
    out.push({
      kind: 'custom',
      id: Number(tool.id),
      refKey: `cust:${tool.id}`,
      name: tool.name || tool.slug,
      description: tool.description || '',
      // For custom tools, surface slug as a font-mono metadata hint —
      // the description carries the human-facing intent.
      metadata: tool.slug || '',
      accessType: tool.access_type || '',
    })
  }
  return out
})

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return rows.value
  return rows.value.filter(
    (r) =>
      r.name.toLowerCase().includes(q) ||
      r.description.toLowerCase().includes(q) ||
      (r.metadata && String(r.metadata).toLowerCase().includes(q))
  )
})

// agent.tools is an array of ToolRefs. We index it for O(1) lookup
// during toggle + selection-state queries.
const selectedIndex = computed(() => {
  const m = new Map()
  for (const r of agent.value?.tools || []) {
    if (!r) continue
    if (r.kind === 'internal') m.set(`int:${String(r.id)}`, r)
    else if (r.kind === 'custom') m.set(`cust:${Number(r.id)}`, r)
  }
  return m
})

function isSelected(row) {
  return selectedIndex.value.has(row.refKey)
}

function toggle(row, on) {
  if (!agent.value) return
  const tools = (agent.value.tools || []).filter((r) => {
    if (!r) return false
    const key = r.kind === 'internal' ? `int:${String(r.id)}` : `cust:${Number(r.id)}`
    return key !== row.refKey
  })
  if (on) {
    if (row.kind === 'internal') tools.push({ kind: 'internal', id: String(row.id) })
    else tools.push({ kind: 'custom', id: Number(row.id) })
  }
  agent.value.tools = tools
}

const enabledCount = computed(() =>
  rows.value.filter((r) => isSelected(r)).length
)
</script>
