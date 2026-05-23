<!--
  AgentTabHistory — past runs for this agent.

  Sources GET /api/v1/ai/agents/:id/runs (handleListAgentRuns).
  Cursor pagination via (before_run_at, before_id) — pass values from
  the last row of the current page to fetch the next.

  Row click expands inline, rendering AgentRunResultPanel against the
  normalized run shape — same component the Preview panel uses for
  live runs and ComposerTab uses for past composer runs.
-->
<template>
  <div class="px-6 py-6 max-w-5xl">
    <div class="flex items-center justify-between gap-4 mb-3">
      <h2 class="text-lg font-semibold">{{ t('admin.ai.agent.tab.history') }}</h2>
      <span v-if="rows.length > 0" class="text-xs text-muted-foreground">
        {{ rows.length }} {{ rows.length === 1 ? 'run' : 'runs' }}
      </span>
    </div>

    <!-- Filter row -->
    <div class="flex flex-wrap items-center gap-2 mb-3">
      <Input
        v-model="search"
        type="text"
        class="flex-1 min-w-[200px]"
        :placeholder="t('admin.ai.agent.history.searchPlaceholder')"
        @input="onFilterChange"
      />
      <select
        v-model="statusFilter"
        class="border border-input bg-background rounded-md px-2 py-1.5 text-sm"
        @change="onFilterChange"
      >
        <option value="">{{ t('admin.ai.agent.history.allStatuses') }}</option>
        <option value="complete">{{ t('admin.ai.agent.history.statusComplete') }}</option>
        <option value="partial">{{ t('admin.ai.agent.history.statusPartial') }}</option>
        <option value="max_iterations">{{ t('admin.ai.agent.history.statusMaxIterations') }}</option>
        <option value="failed">{{ t('admin.ai.agent.history.statusFailed') }}</option>
        <option value="refused">{{ t('admin.ai.agent.history.statusRefused') }}</option>
      </select>
    </div>

    <div v-if="loading && rows.length === 0" class="text-sm text-muted-foreground">
      {{ t('globals.messages.loading') }}
    </div>
    <div
      v-else-if="!loading && rows.length === 0"
      class="text-center text-sm text-muted-foreground py-12 border border-dashed border-input rounded-md"
    >
      {{ t('admin.ai.agent.history.empty') }}
    </div>
    <div v-else class="border border-input rounded-md overflow-hidden">
      <!-- Header -->
      <div class="grid grid-cols-[140px_110px_70px_120px_minmax(0,1fr)] gap-3 px-4 py-2 text-xs text-muted-foreground bg-muted/40 border-b border-input">
        <span>{{ t('globals.terms.when') }}</span>
        <span>{{ t('globals.terms.status') }}</span>
        <span class="text-right">{{ t('globals.terms.iters') }}</span>
        <span class="text-right">{{ t('globals.terms.tokens') }}</span>
        <span>{{ t('globals.terms.draft') }}</span>
      </div>

      <ul class="divide-y divide-input">
        <template v-for="row in rows" :key="row.id">
          <li
            class="grid grid-cols-[140px_110px_70px_120px_minmax(0,1fr)] gap-3 px-4 py-2 items-center text-sm cursor-pointer hover:bg-accent/30 transition-colors"
            @click="toggleExpand(row.id)"
          >
            <span class="text-muted-foreground">{{ formatTime(row.run_at) }}</span>
            <span>
              <Badge :variant="badgeVariantFor(row.status)" class="text-xs">
                {{ row.status }}
              </Badge>
            </span>
            <span class="text-right text-muted-foreground font-mono text-xs">
              {{ row.usage?.iterations ?? '—' }}
            </span>
            <span class="text-right text-muted-foreground font-mono text-xs">
              {{ row.usage?.input_tokens ?? 0 }}/{{ row.usage?.output_tokens ?? 0 }}
            </span>
            <span class="truncate text-muted-foreground">
              {{ snippet(row.draft) }}
            </span>
          </li>
          <li v-if="expanded.has(row.id)" class="px-4 py-3 bg-muted/20 border-t border-input">
            <AgentRunResultPanel :result="normalize(row)" :verbose="true" />
          </li>
        </template>
      </ul>

      <!-- Cursor pagination footer -->
      <div v-if="hasMore" class="px-4 py-2 border-t border-input text-center">
        <button
          type="button"
          class="text-xs text-muted-foreground hover:text-foreground transition-colors"
          :disabled="loading"
          @click="loadMore"
        >
          {{ loading
            ? (t('globals.messages.loading'))
            : (t('globals.messages.loadOlder')) }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Badge } from '@shared-ui/components/ui/badge'
import AgentRunResultPanel from '@/features/ai/shared/RunResultPanel.vue'
import { normalizeExecution } from '@/features/ai/shared/normalizeRunResult.js'
import api from '@/api'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { inject } from 'vue'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agentId = computed(() => edit?.agent.value?.id)

const search = ref('')
const statusFilter = ref('')
const rows = ref([])
const loading = ref(false)
const expanded = ref(new Set())
const hasMore = ref(false)

let filterDebounce = null
function onFilterChange() {
  clearTimeout(filterDebounce)
  filterDebounce = setTimeout(() => load(true), 250)
}

async function load(reset = false) {
  if (!agentId.value) return
  if (reset) {
    rows.value = []
    expanded.value = new Set()
  }
  loading.value = true
  try {
    const params = { limit: 20 }
    if (statusFilter.value) params.status = statusFilter.value
    if (search.value.trim()) params.q = search.value.trim()
    if (!reset && rows.value.length > 0) {
      const last = rows.value[rows.value.length - 1]
      params.before_run_at = last.run_at
      params.before_id = last.id
    }
    const resp = await api.listAIAgentRuns(agentId.value, params)
    const fresh = resp.data?.data?.executions || []
    rows.value = reset ? fresh : [...rows.value, ...fresh]
    hasMore.value = fresh.length === params.limit
  } catch {
    // Endpoint missing or transient — leave the list as-is. Empty
    // state renders when rows.value === [].
    if (reset) hasMore.value = false
  } finally {
    loading.value = false
  }
}

function loadMore() {
  load(false)
}

function toggleExpand(id) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}

function normalize(row) {
  return normalizeExecution(row)
}

function snippet(s) {
  if (!s) return '—'
  const oneLine = String(s).replace(/\s+/g, ' ').trim()
  return oneLine.length > 100 ? oneLine.slice(0, 100) + '…' : oneLine
}

function badgeVariantFor(status) {
  if (status === 'complete') return 'default'
  if (status === 'failed' || status === 'refused') return 'destructive'
  return 'secondary'
}

function formatTime(iso) {
  if (!iso) return '—'
  try {
    const d = new Date(iso)
    return d.toLocaleString()
  } catch {
    return '—'
  }
}

onMounted(() => load(true))
</script>
