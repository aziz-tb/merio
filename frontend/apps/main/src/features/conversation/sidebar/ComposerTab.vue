<template>
  <div class="flex items-center justify-between gap-2 p-4">
    <div class="w-full">
      <SelectCombobox
        v-model="adHocAgentId"
        :items="adHocAgentOptions"
        :placeholder="$t('agent.searchAgent')"
      />
    </div>
    <Button
      variant="outline"
      size="icon"
      class="size-9 p-4"
      :disabled="validating"
      :title="$t('agent.validation.revalidate')"
      @click="triggerValidation"
    >
      <Loader2 v-if="validating" class="size-4 animate-spin" />
      <RefreshCw v-else class="size-4" />
      <span class="sr-only">{{ $t('agent.validation.revalidate') }}</span>
    </Button>
  </div>
  <div class="p-4 space-y-4">
    <div v-if="loading" class="text-xs text-muted-foreground">
      {{ $t('globals.messages.loading') }}
    </div>

    <div
      v-else-if="!allAgents.length"
      class="rounded-md border border-dashed border-input bg-muted/20 px-4 py-8 text-center text-sm text-muted-foreground"
    >
      {{ $t('composer.empty') }}
    </div>

    <div v-else class="space-y-3">
      <div
        v-if="!latestValidation && !matchingAgents.length"
        class="rounded-md border border-dashed border-input bg-muted/20 px-4 py-6 text-center"
      >
        <Button variant="ghost" size="sm" :disabled="validating" @click="triggerValidation">
          <Loader2 v-if="validating" class="mr-2 size-4 animate-spin" />
          <RefreshCw v-else class="mr-2 size-4" />
          {{ $t('agent.validation.validateNow') }}
        </Button>
      </div>

      <div
        v-else-if="latestValidation && !matchingAgents.length"
        class="rounded-md border border-dashed border-input bg-muted/20 px-4 py-6 text-center text-sm text-muted-foreground"
      >
        {{ $t('agent.validation.noMatches') }}
      </div>

      <div
        v-for="agent in visibleAgents"
        :key="agent.id"
        class="rounded-md border border-input bg-card"
      >
        <div class="flex items-center justify-between gap-2 px-3 py-2">
          <button
            type="button"
            class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm font-medium"
            @click="toggleExpanded(agent.id)"
          >
            <span class="shrink-0 text-muted-foreground">
              {{ isExpanded(agent.id) ? '▾' : '▸' }}
            </span>
            <span class="truncate">{{ agent.name }}</span>
            <Badge
              v-if="rowsByAgent[agent.id]?.outcome?.outcome === 'sent'"
              variant="default"
              class="shrink-0 text-xs"
            >
              {{ $t('composer.sent') }}
            </Badge>
          </button>

          <!-- Busy: spinner -->
          <Button
            v-if="isAgentBusy(agent.id)"
            size="icon"
            variant="secondary"
            disabled
          >
            <Loader2 class="h-4 w-4 animate-spin" />
          </Button>

          <!-- First run: start the agent -->
          <Tooltip v-else-if="!rowsByAgent[agent.id]">
            <TooltipTrigger as-child>
              <Button
                size="icon"
                variant="secondary"
                :disabled="isRunActionDisabled(agent.id)"
                @click="runAgent(agent)"
              >
                <Play class="h-4 w-4" />
                <span class="sr-only">{{ $t('composer.runAgent') }}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ $t('composer.runAgent') }}</TooltipContent>
          </Tooltip>

          <!-- Completed run with a trace: toggle trace vs draft view.
               Stays available after the reply is sent (read-only inspection);
               re-running now lives only on the in-panel Regenerate action. -->
          <Tooltip v-else-if="hasTrace(agent.id)">
            <TooltipTrigger as-child>
              <Button
                size="icon"
                variant="ghost"
                @click="toggleTraceView(agent.id)"
              >
                <History
                  class="h-4 w-4"
                  :class="isTraceView(agent.id) ? 'text-foreground' : 'text-muted-foreground'"
                />
                <span class="sr-only">
                  {{ isTraceView(agent.id) ? $t('composer.hideActivity') : $t('composer.showActivity') }}
                </span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {{ isTraceView(agent.id) ? $t('composer.hideActivity') : $t('composer.showActivity') }}
            </TooltipContent>
          </Tooltip>
        </div>

        <div
          v-if="isExpanded(agent.id) && showPanelForAgent(agent.id)"
          class="border-t border-input px-3 pb-3 pt-3"
        >
          <AgentRunResultPanel
            :result="panelResult(agent.id)"
            :is-stale="rowsByAgent[agent.id]?.is_stale"
            :stream-events="streamEventsFor(agent.id)"
            :stream-status="streamStatusFor(agent.id)"
            :stream-error="streamErrorFor(agent.id)"
            :is-streaming="isAgentStreaming(agent.id)"
            :activity-view="isTraceView(agent.id)"
            :hide-activity-toggle="true"
          >
            <template
              v-if="shouldShowActions(agent.id)"
              #actions
            >
              <Button
                v-if="shouldShowRetry(agent.id)"
                size="sm"
                variant="secondary"
                :disabled="hasActiveRun"
                @click="runAgent(agent)"
              >
                <RotateCcw class="mr-1 h-3 w-3" />
                {{ $t('composer.retry') }}
              </Button>

              <template v-else-if="rowsByAgent[agent.id]?.outcome?.outcome !== 'sent'">
                <Button
                  v-if="canSend(agent.id)"
                  size="sm"
                  :disabled="
                    sendingExecutionId === rowsByAgent[agent.id].agent_execution.id ||
                    hasActiveRun ||
                    !rowsByAgent[agent.id].agent_execution.id
                  "
                  @click="sendReply(agent, rowsByAgent[agent.id])"
                >
                  <Loader2
                    v-if="sendingExecutionId === rowsByAgent[agent.id].agent_execution.id"
                    class="mr-1 h-3 w-3 animate-spin"
                  />
                  <Send v-else class="mr-1 h-3 w-3" />
                  {{ $t('composer.sendReply') }}
                </Button>

                <Button
                  v-if="canEdit(agent.id)"
                  size="sm"
                  variant="secondary"
                  :disabled="hasActiveRun"
                  @click="editReply(rowsByAgent[agent.id])"
                >
                  <Pencil class="mr-1 h-3 w-3" />
                  {{ $t('composer.editReply') }}
                </Button>

                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      size="icon"
                      variant="secondary"
                      :disabled="isRunActionDisabled(agent.id)"
                      @click="runAgent(agent)"
                    >
                      <RotateCcw class="h-4 w-4" />
                      <span class="sr-only">{{ $t('composer.regenerate') }}</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{{ $t('composer.regenerate') }}</TooltipContent>
                </Tooltip>
              </template>
            </template>
          </AgentRunResultPanel>
        </div>
      </div>

    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, shallowRef, toRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ClockRotateLeft as History, Loader as Loader2, Pencil, Play, ArrowsRotate as RefreshCw, ArrowRotateLeft as RotateCcw, PaperPlane as Send } from '@qall/icons'
import { Badge } from '@shared-ui/components/ui/badge'
import { Button } from '@shared-ui/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@shared-ui/components/ui/tooltip'
import AgentRunResultPanel from '@/features/ai/shared/RunResultPanel.vue'
import { normalizeExecution } from '@/features/ai/shared/normalizeRunResult.js'
import { useAgentRunStream } from '@/composables/useAgentRunStream'
import { useWorkflowCompletion } from '@/composables/useWorkflowCompletion'
import { useConversationStore } from '@/stores/conversation'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { MACRO_CONTEXT } from '@/constants/conversation'
import SelectCombobox from '@/components/combobox/SelectCombobox.vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  uuid: { type: String, required: true },
})

const { t } = useI18n()
const emitter = useEmitter()
const conversationStore = useConversationStore()

const loading = ref(false)
const allAgents = ref([])
const matchingAgents = ref([])
const latestValidation = ref(null)
const validating = ref(false)
const adHocAgentId = ref(null)
const adHocAgentIds = ref(new Set())
const rowsByAgent = ref({})
const expandedAgentIds = ref(new Set())
const sendingExecutionId = ref(null)
const activeRun = shallowRef(null)

const activeStreamStatus = computed(() => {
  const r = activeRun.value
  if (!r) return 'idle'
  return r.stream?.status?.value ?? 'idle'
})
const activeStreamEvents = computed(() => {
  const r = activeRun.value
  if (!r) return []
  return r.stream?.events?.value ?? []
})
const activeStreamError = computed(() => {
  const r = activeRun.value
  if (!r) return ''
  return r.stream?.error?.value ?? ''
})
const activeAgentId = computed(() => activeRun.value?.agentId || 0)
const hasActiveRun = computed(() =>
  ['queued', 'starting', 'running', 'disconnected'].includes(activeStreamStatus.value)
)

const isExpanded = (id) => expandedAgentIds.value.has(id)
const toggleExpanded = (id) => {
  const next = new Set(expandedAgentIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedAgentIds.value = next
}

// Per-agent toggle for the card-header trace view: when on, the panel shows
// the run's tool+thinking trace in place of the draft.
const traceViewAgentIds = ref(new Set())
const isTraceView = (id) => traceViewAgentIds.value.has(id)
const hasTrace = (id) => Boolean(rowsByAgent.value[id]?.agent_execution?.trace?.length)
const toggleTraceView = (id) => {
  if (!expandedAgentIds.value.has(id)) {
    const expanded = new Set(expandedAgentIds.value)
    expanded.add(id)
    expandedAgentIds.value = expanded
  }
  const next = new Set(traceViewAgentIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  traceViewAgentIds.value = next
}

const normalize = (execution) => normalizeExecution(execution)
const panelResult = (agentId) =>
  rowsByAgent.value[agentId]?.agent_execution
    ? normalize(rowsByAgent.value[agentId].agent_execution)
    : null

const streamStatusFor = (agentId) =>
  activeAgentId.value === agentId ? activeStreamStatus.value : 'idle'
const streamEventsFor = (agentId) =>
  activeAgentId.value === agentId ? activeStreamEvents.value : []
const streamErrorFor = (agentId) =>
  activeAgentId.value === agentId ? activeStreamError.value : ''

const isAgentBusy = (agentId) =>
  hasActiveRun.value && activeAgentId.value === agentId
const isAgentStreaming = (agentId) =>
  activeAgentId.value === agentId &&
  ['queued', 'starting', 'running', 'disconnected', 'failed'].includes(activeStreamStatus.value)

const isRunActionDisabled = () => hasActiveRun.value
const showPanelForAgent = (agentId) =>
  Boolean(rowsByAgent.value[agentId] || activeRun.value?.agentId === agentId)

const shouldShowRetry = (agentId) => {
  if (streamStatusFor(agentId) === 'failed') return true
  const result = panelResult(agentId)
  return result?.status === 'incomplete'
}

const canSend = (agentId) => {
  const row = rowsByAgent.value[agentId]
  const result = panelResult(agentId)
  return Boolean(row?.agent_execution?.id && result?.status === 'complete' && result?.draft)
}

const canEdit = (agentId) => {
  const result = panelResult(agentId)
  return Boolean(result?.status === 'complete' && result?.draft)
}

const shouldShowActions = (agentId) => {
  if (streamStatusFor(agentId) === 'failed') return true
  if (!rowsByAgent.value[agentId]) return false
  if (rowsByAgent.value[agentId].outcome?.outcome === 'sent') return false
  return true
}

const matchingAgentIDs = computed(() => new Set(matchingAgents.value.map((agent) => agent.id)))
const visibleAgents = computed(() => {
  const seen = new Set()
  const out = []
  for (const agent of matchingAgents.value) {
    seen.add(agent.id)
    out.push(agent)
  }
  for (const agent of allAgents.value) {
    if (seen.has(agent.id)) continue
    if (
      adHocAgentIds.value.has(agent.id) ||
      rowsByAgent.value[agent.id] ||
      activeRun.value?.agentId === agent.id
    ) {
      seen.add(agent.id)
      out.push(agent)
    }
  }
  return out
})

const adHocAgentOptions = computed(() =>
  allAgents.value
    .filter((agent) => !matchingAgentIDs.value.has(agent.id))
    .map((agent) => ({ label: agent.name, value: agent.id }))
)

const agentMetaFor = (agentId, fallback = {}) =>
  allAgents.value.find((agent) => agent.id === agentId) ||
  rowsByAgent.value[agentId]?.agent || {
    id: agentId,
    name: fallback.name || `Agent #${agentId}`,
    slug: fallback.slug || '',
    enabled: true,
    updated_at: fallback.updated_at || null,
  }

const buildLatestRow = (agentMeta, execution, outcome = null, isStale = false) => ({
  agent_execution: execution,
  outcome,
  agent: {
    id: agentMeta.id,
    name: agentMeta.name,
    slug: agentMeta.slug,
    enabled: agentMeta.enabled,
    updated_at: agentMeta.updated_at,
  },
  is_stale: isStale,
})

const buildExecutionFromRunResult = (agent, executionId, result) => ({
  id: executionId,
  conversation_id: conversationID.value,
  agent_id: agent.id,
  status: result.status,
  draft: result.draft,
  trace: result.trace || [],
  usage: result.usage || {},
  duration_ms: result.duration_ms ?? 0,
  agent_updated_at: agent.updated_at,
})

const clearActiveRun = () => {
  if (activeRun.value?.stopWatch) {
    activeRun.value.stopWatch()
  }
  if (activeRun.value?.stream) {
    activeRun.value.stream.disconnect()
  }
  activeRun.value = null
}

const hydrateExecutionRow = async (executionId) => {
  if (!executionId) return
  const resp = await api.getAgentExecution(executionId)
  const execution = resp.data?.data
  if (!execution) return
  const agentMeta = agentMetaFor(execution.agent_id, { updated_at: execution.agent_updated_at })
  rowsByAgent.value = {
    ...rowsByAgent.value,
    [execution.agent_id]: buildLatestRow(agentMeta, execution),
  }
  const nextExpanded = new Set(expandedAgentIds.value)
  nextExpanded.add(execution.agent_id)
  expandedAgentIds.value = nextExpanded
}

const attachRunStream = (agent, accepted) => {
  if (!accepted?.trace_id) return
  if (activeRun.value?.traceId === accepted.trace_id) return

  clearActiveRun()

  const stream = useAgentRunStream(accepted.trace_id)
  const run = {
    agentId: agent.id,
    traceId: accepted.trace_id,
    stream,
    stopWatch: null,
  }
  run.stopWatch = watch(
    [stream.status, stream.finalResult, stream.executionId, stream.error],
    async ([nextStatus, nextResult, nextExecutionId, nextError]) => {
      if (nextStatus === 'completed') {
        if (nextExecutionId && nextResult) {
          rowsByAgent.value = {
            ...rowsByAgent.value,
            [agent.id]: buildLatestRow(
              agent,
              buildExecutionFromRunResult(agent, nextExecutionId, nextResult)
            ),
          }
          const nextExpanded = new Set(expandedAgentIds.value)
          nextExpanded.add(agent.id)
          expandedAgentIds.value = nextExpanded
        } else if (nextExecutionId) {
          await hydrateExecutionRow(nextExecutionId)
        }
        clearActiveRun()
        return
      }
      if (nextStatus === 'failed') {
        const nextExpanded = new Set(expandedAgentIds.value)
        nextExpanded.add(agent.id)
        expandedAgentIds.value = nextExpanded
        emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
          variant: 'destructive',
          description: nextError || t('globals.messages.somethingWentWrong'),
        })
      }
    }
  )
  activeRun.value = run
  stream.connect()
}

const restoreConversationRunState = async () => {
  if (!props.uuid) return
  try {
    const resp = await api.getConversationAgentRunState(props.uuid)
    const state = resp.data?.data
    if (!state) return

    if (state.state === 'in_flight' && state.trace_id && state.agent_id) {
      attachRunStream(agentMetaFor(state.agent_id), {
        trace_id: state.trace_id,
      })
      return
    }
    if (state.state === 'completed' && state.execution_id) {
      await hydrateExecutionRow(state.execution_id)
    }
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message,
    })
  }
}

const buildRunPayload = () => {
  const all = (conversationStore.conversationMessages || []).slice()
  all.sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
  const visible = all.filter(
    (m) => m && (m.type === 'incoming' || m.type === 'outgoing') && m.private !== true
  )
  if (!visible.length) return { inbound_message: '', conversation_history: [] }

  let inboundIdx = -1
  for (let i = visible.length - 1; i >= 0; i -= 1) {
    if (visible[i].type === 'incoming') {
      inboundIdx = i
      break
    }
  }
  if (inboundIdx === -1) inboundIdx = visible.length - 1
  const inboundMsg = visible[inboundIdx]
  const inbound_message =
    (inboundMsg.text_content && inboundMsg.text_content.trim()) ||
    stripHTML(inboundMsg.content || '')

  const others = visible.slice(0, inboundIdx).concat(visible.slice(inboundIdx + 1))
  const capped = others.slice(-20)
  const conversation_history = capped.map((m) => ({
    sender: m.type === 'incoming' ? 'customer' : 'agent',
    text: (m.text_content && m.text_content.trim()) || stripHTML(m.content || ''),
  }))
  return { inbound_message, conversation_history }
}

const stripHTML = (html) =>
  String(html || '')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/p>/gi, '\n')
    .replace(/<[^>]+>/g, '')
    .replace(/&nbsp;/g, ' ')
    .trim()

const conversationID = computed(() => conversationStore.current?.id || 0)

const applyValidation = (operation) => {
  latestValidation.value = operation || null
  const matchingIDs = operation?.payload?.matching_agent_ids || []
  matchingAgents.value = allAgents.value.filter((agent) => matchingIDs.includes(agent.id))
}

const fetchLatestValidation = async () => {
  try {
    const validationResp = await api.getLatestAgentValidation(props.uuid)
    applyValidation(validationResp.data?.data)
  } catch (error) {
    if (error?.response?.status === 404) {
      applyValidation(null)
      return
    }
    console.warn('Failed to fetch latest agent validation', error)
  }
}

const triggerValidation = async () => {
  if (!props.uuid || validating.value) return
  validating.value = true
  try {
    await api.validateConversationAgents(props.uuid)
  } catch (error) {
    const status = error?.response?.status
    const description =
      status === 409
        ? t('agent.validation.alreadyRunning')
        : status === 503
          ? t('agent.validation.notAvailable')
          : handleHTTPError(error).message
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description,
    })
    validating.value = false
  }
}

const handleAgentValidationCompleted = async () => {
  try {
    await fetchLatestValidation()
  } finally {
    validating.value = false
  }
}

const refresh = async () => {
  if (!props.uuid) return
  loading.value = true
  try {
    const [latestResp, agentsResp] = await Promise.all([
      api.getLatestAgentExecutions(props.uuid),
      api.getAIAgents({ enabled: true, page: 1, page_size: 100 }),
    ])
    const latestRows = latestResp.data?.data?.executions || []
    rowsByAgent.value = Object.fromEntries(latestRows.map((row) => [row.agent.id, row]))

    const enabled = agentsResp.data?.data?.results || []
    allAgents.value = enabled.sort((a, b) => a.name.localeCompare(b.name))
    await fetchLatestValidation()

    const nextExpanded = new Set(expandedAgentIds.value)
    for (const row of latestRows) {
      nextExpanded.add(row.agent.id)
    }
    expandedAgentIds.value = nextExpanded
    await restoreConversationRunState()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message,
    })
  } finally {
    loading.value = false
  }
}

const runAgent = async (agent) => {
  if (!conversationID.value) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('composer.noConversation'),
    })
    return
  }
  if (hasActiveRun.value) return

  try {
    const { inbound_message, conversation_history } = buildRunPayload()
    if (!inbound_message) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('composer.noInbound'),
      })
      return
    }
    const resp = await api.runAIAgent(agent.id, {
      inbound_message,
      conversation_id: conversationID.value,
      conversation_history,
    })
    const data = resp.data?.data
    if (!data || !data.trace_id) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('admin.ai.agent.testPanel.emptyResponse'),
      })
      return
    }
    const nextExpanded = new Set(expandedAgentIds.value)
    nextExpanded.add(agent.id)
    expandedAgentIds.value = nextExpanded
    attachRunStream(agent, data)
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message,
    })
  }
}

const sendReply = async (agent, row) => {
  if (!row?.agent_execution?.id) return
  const executionId = row.agent_execution.id
  sendingExecutionId.value = executionId
  try {
    const resp = await api.sendAgentExecution(props.uuid, executionId)
    const data = resp.data?.data || {}
    if (data.success) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        description: t('composer.sentToast'),
      })
    } else if (data.message_sent && !data.ticket_closed) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('composer.sentCloseFailedToast'),
      })
    } else {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: data.error || t('globals.messages.somethingWentWrong'),
      })
    }
    await refresh()
  } catch (error) {
    const httpErr = handleHTTPError(error)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: httpErr.message,
    })
    if (error?.response?.status === 409) await refresh()
  } finally {
    sendingExecutionId.value = null
  }
}

const editReply = (row) => {
  const draft = row?.agent_execution?.draft || ''
  const html = plainTextToHTML(draft)
  conversationStore.setMacro(
    {
      id: `composer:${row.agent_execution.id}:${Date.now()}`,
      message_content: html,
    },
    MACRO_CONTEXT.REPLY
  )
  emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
    description: t('composer.editToast'),
  })
}

const plainTextToHTML = (s) => {
  if (!s) return ''
  const trimmed = String(s).trim().replace(/\r\n?/g, '\n')
  if (!trimmed) return ''
  return trimmed
    .split('\n\n')
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => '<p>' + escapeHTML(p).replace(/\n/g, '<br>') + '</p>')
    .join('')
}

const escapeHTML = (s) =>
  String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

watch(adHocAgentId, (id) => {
  if (!id) return
  const agent = allAgents.value.find((a) => String(a.id) === String(id))
  if (!agent) return
  const next = new Set(adHocAgentIds.value)
  next.add(agent.id)
  adHocAgentIds.value = next
  adHocAgentId.value = null
  runAgent(agent)
})

useWorkflowCompletion('AGENT_VALIDATION_COMPLETED', toRef(props, 'uuid'), handleAgentValidationCompleted)

onMounted(refresh)
onUnmounted(() => {
  clearActiveRun()
})

watch(
  () => props.uuid,
  (next, prev) => {
    if (next && next !== prev) {
      clearActiveRun()
      latestValidation.value = null
      matchingAgents.value = []
      adHocAgentId.value = null
      adHocAgentIds.value = new Set()
      traceViewAgentIds.value = new Set()
      validating.value = false
      refresh()
    }
  }
)
</script>
