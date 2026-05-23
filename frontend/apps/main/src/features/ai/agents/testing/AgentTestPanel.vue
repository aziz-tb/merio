<!--
  AgentTestPanel — inline run panel for the agent edit form.

  Renders only on existing agents (the agent must have an id). Posts to
  /api/v1/ai/agents/:id/run and renders the runner's full RunResponse:
  status badge, final draft (markdown), per-turn trace timeline, and
  aggregate metrics. Trace turns are expandable; each turn shows the
  stop_reason badge + latency + token usage and lists any tool calls
  with their input / output / error.

  This is the validation surface for the F7+F5 foundation milestone —
  without it the only way to see the runner's behavior is curl + log
  scraping.
-->
<template>
  <div class="border border-input rounded-lg p-5 space-y-4 bg-muted/20">
    <div class="flex items-center justify-between">
      <h3 class="text-base font-semibold text-foreground">
        {{ $t('admin.ai.agent.testPanel.title') }}
      </h3>
      <Badge v-if="!agent.enabled" variant="secondary">
        {{ $t('globals.terms.disabled') }}
      </Badge>
    </div>

    <p class="text-sm text-muted-foreground">
      {{ $t('admin.ai.agent.testPanel.help') }}
    </p>

    <!-- Inputs -->
    <div class="space-y-3">
      <div class="space-y-1.5">
        <Label class="text-muted-foreground">
          {{ $t('admin.ai.agent.testPanel.inboundMessage') }}
          <span class="text-destructive">*</span>
        </Label>
        <textarea
          v-model="inboundMessage"
          rows="4"
          class="w-full border border-input rounded-md p-2 text-sm font-mono bg-background"
          :placeholder="$t('admin.ai.agent.testPanel.inboundPlaceholder')"
        />
      </div>

      <div class="grid grid-cols-2 gap-4">
        <div class="space-y-1.5">
          <RealConversationSelect v-model="pickedConversationId" />
          <details class="text-xs text-muted-foreground">
            <summary class="cursor-pointer">
              {{ $t('admin.ai.agent.testPanel.conversationId') }}
            </summary>
            <Input
              class="mt-1"
              type="number"
              min="0"
              step="1"
              :model-value="conversationId"
              @input="(e) => (conversationId = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
              placeholder="0"
            />
          </details>
        </div>
        <div class="space-y-1.5">
          <Label class="text-muted-foreground">
            {{ $t('admin.ai.agent.testPanel.legitimationLevel') }}
          </Label>
          <Input
            type="number"
            min="0"
            step="1"
            :model-value="legitimationLevel"
            @input="(e) => (legitimationLevel = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
          />
        </div>
      </div>

      <details class="border border-input rounded-md p-3">
        <summary class="text-sm cursor-pointer text-muted-foreground">
          {{ $t('admin.ai.agent.testPanel.advanced') }}
        </summary>
        <div class="grid grid-cols-3 gap-4 mt-3">
          <div class="space-y-1.5">
            <Label class="text-muted-foreground text-xs">
              {{ $t('admin.ai.agent.testPanel.maxIterations') }}
            </Label>
            <Input
              type="number"
              min="0"
              step="1"
              :model-value="maxIterations"
              @input="(e) => (maxIterations = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
              placeholder="0 = use agent's"
            />
          </div>
          <div class="space-y-1.5">
            <Label class="text-muted-foreground text-xs">
              {{ $t('admin.ai.agent.testPanel.totalTokenBudget') }}
            </Label>
            <Input
              type="number"
              min="0"
              step="1000"
              :model-value="totalTokenBudget"
              @input="(e) => (totalTokenBudget = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
              placeholder="0 = 50000"
            />
          </div>
          <div class="space-y-1.5">
            <Label class="text-muted-foreground text-xs">
              {{ $t('admin.ai.agent.testPanel.deadlineMs') }}
            </Label>
            <Input
              type="number"
              min="0"
              step="1000"
              :model-value="deadlineMs"
              @input="(e) => (deadlineMs = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
              placeholder="0 = 60000"
            />
          </div>
        </div>
      </details>
    </div>

    <Button
      @click="runAgent"
      :is-loading="isRunning"
      :disabled="isRunning || !inboundMessage.trim()"
      class="w-full"
    >
      {{ $t('admin.ai.agent.testPanel.run') }}
    </Button>

    <div v-if="normalizedResult || activeStream" class="space-y-4 pt-3 border-t border-input">
      <AgentRunResultPanel
        :result="normalizedResult"
        :is-stale="false"
        :stream-events="activeStream?.events.value || []"
        :stream-status="activeStream?.status.value || 'idle'"
        :stream-error="activeStream?.error.value || ''"
        :is-streaming="['queued', 'starting', 'running', 'disconnected', 'failed'].includes(activeStream?.status.value || 'idle')"
      >
        <template v-if="lastResult?.drops" #actions>
          <div class="text-xs text-muted-foreground italic">
            {{ $t('admin.ai.agent.testPanel.drops') }}:
            <span v-for="(reason, idx) in lastResult.drops" :key="idx">
              tools[{{ idx }}] — {{ reason }};
            </span>
          </div>
        </template>
      </AgentRunResultPanel>
    </div>
  </div>
</template>

<script setup>
import { ref, shallowRef, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'
import RealConversationSelect from '@/features/ai/agents/testing/ConversationSelect.vue'
import AgentRunResultPanel from '@/features/ai/shared/RunResultPanel.vue'
import { normalizeRunResponse } from '@/features/ai/shared/normalizeRunResult.js'
import { useEmitter } from '@/composables/useEmitter'
import { useAgentRunStream } from '@/composables/useAgentRunStream'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  agent: { type: Object, required: true }
})

const { t } = useI18n()
const emitter = useEmitter()

const inboundMessage = ref('')
const conversationId = ref(0)
// Drives the typeahead picker; selecting a conversation populates
// conversationId. The two stay loosely linked so an admin can still
// type a raw id directly into the advanced details box.
const pickedConversationId = ref(null)
watch(pickedConversationId, (v) => {
  conversationId.value = v || 0
})
const legitimationLevel = ref(0)
const maxIterations = ref(0)
const totalTokenBudget = ref(0)
const deadlineMs = ref(0)
const isRunning = ref(false)
const lastResult = ref(null)
const normalizedResult = computed(() => normalizeRunResponse(lastResult.value))
const activeStream = shallowRef(null)
let stopStreamWatch = null

const clearStream = () => {
  if (stopStreamWatch) {
    stopStreamWatch()
    stopStreamWatch = null
  }
  if (activeStream.value) {
    activeStream.value.disconnect()
    activeStream.value = null
  }
}

async function runAgent() {
  if (!inboundMessage.value.trim()) return
  try {
    isRunning.value = true
    lastResult.value = null
    const body = {
      inbound_message: inboundMessage.value,
      conversation_id: conversationId.value || 0,
      legitimation_level: legitimationLevel.value || 0
    }
    if (maxIterations.value > 0) body.max_iterations = maxIterations.value
    if (totalTokenBudget.value > 0) body.total_token_budget = totalTokenBudget.value
    if (deadlineMs.value > 0) body.deadline_ms = deadlineMs.value

    const r = await api.runAIAgent(props.agent.id, body)
    const accepted = r.data?.data ?? r.data
    if (!accepted?.trace_id) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('admin.ai.agent.testPanel.emptyResponse')
      })
      return
    }

    clearStream()
    const stream = useAgentRunStream(accepted.trace_id)
    activeStream.value = stream
    stopStreamWatch = watch(
      [stream.status, stream.finalResult, stream.error],
      ([nextStatus, nextResult, nextError]) => {
        if (nextStatus === 'completed') {
          lastResult.value = nextResult
          isRunning.value = false
          clearStream()
          return
        }
        if (nextStatus === 'failed') {
          isRunning.value = false
          emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
            variant: 'destructive',
            description: nextError || t('globals.messages.somethingWentWrong')
          })
        }
      }
    )
    stream.connect()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
    isRunning.value = false
    clearStream()
  }
}
</script>
