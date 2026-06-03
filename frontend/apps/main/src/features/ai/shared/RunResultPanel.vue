<template>
  <div class="space-y-4">
    <div
      v-if="isStale"
      class="flex items-center gap-2 rounded-md border border-amber-300/60 bg-amber-50 px-2 py-1.5 text-xs dark:border-amber-700/40 dark:bg-amber-950/30"
    >
      <CircleAlert class="h-3.5 w-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
      <span class="text-amber-900 dark:text-amber-200">
        {{ $t('composer.staleIndicator') }}
      </span>
    </div>

    <Alert
      v-if="streamStatus === 'disconnected'"
      class="border-amber-300/60 bg-amber-50 text-amber-900 dark:border-amber-700/40 dark:bg-amber-950/30 dark:text-amber-200"
    >
      <WifiOff class="h-4 w-4 text-amber-600 dark:text-amber-400" />
      <AlertTitle>{{ $t('composer.reconnecting') }}</AlertTitle>
    </Alert>

    <div
      v-else-if="streamStatus === 'queued'"
      class="flex items-center gap-2 rounded-md bg-muted/30 px-3 py-2 text-sm text-muted-foreground"
    >
      <Loader2 class="h-4 w-4 animate-spin" />
      <span>{{ $t('composer.queuedInQueue') }}</span>
    </div>

    <div v-if="showLiveFeed" class="space-y-2">
      <div v-if="activityItems.length" class="space-y-1.5">
        <div v-for="item in activityItems" :key="item.id" class="space-y-1.5">
          <!-- Tool: expandable variant -->
          <button
            v-if="item.type === 'tool' && hasExpandableContent(item)"
            type="button"
            class="flex w-full items-start gap-3 rounded-md bg-background px-3 py-2 text-left transition-colors hover:bg-accent/30"
            @click="toggleEvent(item.id)"
          >
            <component :is="iconFor(item)" class="mt-0.5 h-4 w-4 shrink-0" :class="iconClassFor(item)" />
            <div class="min-w-0 flex-1 space-y-0.5">
              <div class="flex flex-wrap items-center gap-2">
                <span
                  class="text-sm text-foreground"
                  :class="item.toolKind === 'unknown' ? 'italic text-muted-foreground' : ''"
                >
                  {{ toolLine(item) }}
                </span>
                <Badge
                  v-if="item.toolKind === 'custom'"
                  variant="outline"
                  class="text-[10px] font-normal"
                >
                  {{ $t('globals.terms.custom') }}
                </Badge>
              </div>
              <p v-if="toolSecondaryLine(item)" class="text-xs text-muted-foreground">
                {{ toolSecondaryLine(item) }}
              </p>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
                {{ humanizeDuration(item.durationMs) }}
              </span>
              <component
                :is="expandedEventId === item.id ? ChevronDown : ChevronRight"
                class="h-3.5 w-3.5 text-muted-foreground"
              />
            </div>
          </button>

          <!-- Tool: non-expandable variant -->
          <div
            v-else-if="item.type === 'tool'"
            class="flex items-start gap-3 rounded-md bg-background px-3 py-2"
          >
            <component :is="iconFor(item)" class="mt-0.5 h-4 w-4 shrink-0" :class="iconClassFor(item)" />
            <div class="min-w-0 flex-1 space-y-0.5">
              <div class="flex flex-wrap items-center gap-2">
                <span
                  class="text-sm text-foreground"
                  :class="item.toolKind === 'unknown' ? 'italic text-muted-foreground' : ''"
                >
                  {{ toolLine(item) }}
                </span>
                <Badge
                  v-if="item.toolKind === 'custom'"
                  variant="outline"
                  class="text-[10px] font-normal"
                >
                  {{ $t('globals.terms.custom') }}
                </Badge>
              </div>
              <p v-if="toolSecondaryLine(item)" class="text-xs text-muted-foreground">
                {{ toolSecondaryLine(item) }}
              </p>
            </div>
            <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
              {{ humanizeDuration(item.durationMs) }}
            </span>
          </div>

          <!-- Thinking: expandable (verbose + has content) -->
          <button
            v-else-if="item.type === 'thinking' && canExpandThinking(item)"
            type="button"
            class="flex w-full items-center gap-3 rounded-md bg-background px-3 py-2 text-left transition-colors hover:bg-accent/30"
            @click="toggleThinking(item.id)"
          >
            <component :is="iconFor(item)" class="h-4 w-4 shrink-0" :class="iconClassFor(item)" />
            <span class="min-w-0 flex-1 text-sm text-foreground">{{ simpleLine(item) }}</span>
            <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
              {{ humanizeDuration(item.durationMs) }}
            </span>
            <component
              :is="expandedThinkingId === item.id ? ChevronDown : ChevronRight"
              class="h-3.5 w-3.5 text-muted-foreground"
            />
          </button>

          <!-- Thinking / draft: non-expandable -->
          <div
            v-else
            class="flex items-center gap-3 rounded-md bg-background px-3 py-2"
          >
            <component
              :is="iconFor(item)"
              class="h-4 w-4 shrink-0"
              :class="[iconClassFor(item), item.pending ? 'animate-pulse' : '']"
            />
            <span
              class="min-w-0 flex-1 text-sm text-foreground"
              :class="item.pending ? 'animate-pulse' : ''"
            >
              {{ simpleLine(item) }}
            </span>
            <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
              {{ humanizeDuration(item.durationMs) }}
            </span>
          </div>

          <!-- Tool expanded detail -->
          <div
            v-if="item.type === 'tool' && expandedEventId === item.id"
            class="rounded-md border border-input bg-background px-3 py-2 text-xs space-y-2"
          >
            <div v-if="item.input && Object.keys(item.input).length">
              <div class="mb-1 text-muted-foreground">{{ $t('globals.terms.inputParameters') }}</div>
              <pre class="font-mono whitespace-pre-wrap">{{ shortJson(item.input) }}</pre>
            </div>
            <div v-if="item.error" class="text-destructive">
              {{ item.error }}
            </div>
            <template v-if="item.phase === 'completed' && item.data != null">
              <KnowledgeSearchResults
                v-if="item.toolId === 'kb_search'"
                :results="item.data.results || []"
              />
              <div v-else>
                <div class="mb-1 text-muted-foreground">{{ $t('globals.terms.details') }}</div>
                <pre class="font-mono whitespace-pre-wrap">{{ shortJson(item.data) }}</pre>
              </div>
            </template>
          </div>

          <!-- Thinking expanded content (verbose) -->
          <div
            v-if="item.type === 'thinking' && canExpandThinking(item) && expandedThinkingId === item.id"
            class="rounded-md border border-input bg-background px-3 py-2 space-y-2"
          >
            <MarkdownText v-if="item.thinkingText" :source="item.thinkingText" />
            <p v-if="item.hasRedactedThinking" class="text-xs italic text-muted-foreground">
              {{ $t('composer.redactedThinking') }}
            </p>
          </div>
        </div>
      </div>

      <div
        v-else-if="streamStatus === 'starting' || streamStatus === 'running'"
        class="flex items-center gap-2 rounded-md border border-input bg-background px-3 py-2 text-sm text-muted-foreground"
      >
        <Loader2 class="h-4 w-4 animate-spin" />
        <span>{{ $t('composer.progress.starting') }}</span>
      </div>

      <Alert v-if="streamStatus === 'failed'" variant="destructive">
        <AlertCircle class="h-4 w-4" />
        <AlertTitle>{{ streamError || $t('globals.messages.somethingWentWrong') }}</AlertTitle>
      </Alert>

      <div
        v-if="streamStatus === 'failed' && $slots.actions"
        class="flex flex-wrap items-center gap-2"
      >
        <slot name="actions" />
      </div>
    </div>

    <template v-else-if="result">
      <div
        v-if="hasDraft"
        class="rounded-md bg-background p-3 text-sm leading-relaxed whitespace-pre-wrap"
      >
        {{ result.draft }}
      </div>

      <Alert v-else-if="result.status === 'incomplete'" class="border-input bg-muted/20">
        <CircleAlert class="h-4 w-4" />
        <AlertTitle>{{ $t('composer.couldntFinishReply') }}</AlertTitle>
        <AlertDescription v-if="result.partial_reason">
          {{ result.partial_reason }}
        </AlertDescription>
      </Alert>

      <Alert v-else-if="result.status === 'failed'" variant="destructive">
        <AlertCircle class="h-4 w-4" />
        <AlertTitle>{{ result.partial_reason || $t('globals.messages.somethingWentWrong') }}</AlertTitle>
      </Alert>

      <p v-else-if="result.partial_reason" class="text-sm italic text-muted-foreground">
        {{ result.partial_reason }}
      </p>

      <div
        v-if="$slots.actions || activityItems.length"
        class="flex flex-wrap items-center justify-between gap-2"
      >
        <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2">
          <slot name="actions" />
        </div>
        <Tooltip v-if="activityItems.length">
          <TooltipTrigger as-child>
            <Button
              type="button"
              size="icon"
              variant="ghost"
              @click="showActivity = !showActivity"
            >
              <History class="h-4 w-4" />
              <span class="sr-only">
                {{ showActivity ? $t('composer.hideActivity') : $t('composer.showActivity') }}
              </span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {{ showActivity ? $t('composer.hideActivity') : $t('composer.showActivity') }}
          </TooltipContent>
        </Tooltip>
      </div>

      <div v-if="showActivity && activityItems.length" class="space-y-2 border-t border-input pt-3">
        <p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {{ $t('composer.activity') }}
        </p>
        <div class="space-y-1.5">
          <div v-for="item in activityItems" :key="item.id" class="space-y-1.5">
            <!-- Tool: expandable variant -->
            <button
              v-if="item.type === 'tool' && hasExpandableContent(item)"
              type="button"
              class="flex w-full items-start gap-3 rounded-md border-input bg-background px-3 py-2 text-left transition-colors hover:bg-accent/30"
              @click="toggleEvent(item.id)"
            >
              <component :is="iconFor(item)" class="mt-0.5 h-4 w-4 shrink-0" :class="iconClassFor(item)" />
              <div class="min-w-0 flex-1 space-y-0.5">
                <div class="flex flex-wrap items-center gap-2">
                  <span
                    class="text-sm text-foreground"
                    :class="item.toolKind === 'unknown' ? 'italic text-muted-foreground' : ''"
                  >
                    {{ toolLine(item) }}
                  </span>
                  <Badge
                    v-if="item.toolKind === 'custom'"
                    variant="outline"
                    class="text-[10px] font-normal"
                  >
                    {{ $t('globals.terms.custom') }}
                  </Badge>
                </div>
                <p v-if="toolSecondaryLine(item)" class="text-xs text-muted-foreground">
                  {{ toolSecondaryLine(item) }}
                </p>
              </div>
              <div class="flex items-center gap-2 shrink-0">
                <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
                  {{ humanizeDuration(item.durationMs) }}
                </span>
                <component
                  :is="expandedEventId === item.id ? ChevronDown : ChevronRight"
                  class="h-3.5 w-3.5 text-muted-foreground"
                />
              </div>
            </button>

            <!-- Tool: non-expandable variant -->
            <div
              v-else-if="item.type === 'tool'"
              class="flex items-start gap-3 rounded-md border-input bg-background px-3 py-2"
            >
              <component :is="iconFor(item)" class="mt-0.5 h-4 w-4 shrink-0" :class="iconClassFor(item)" />
              <div class="min-w-0 flex-1 space-y-0.5">
                <div class="flex flex-wrap items-center gap-2">
                  <span
                    class="text-sm text-foreground"
                    :class="item.toolKind === 'unknown' ? 'italic text-muted-foreground' : ''"
                  >
                    {{ toolLine(item) }}
                  </span>
                  <Badge
                    v-if="item.toolKind === 'custom'"
                    variant="outline"
                    class="text-[10px] font-normal"
                  >
                    {{ $t('globals.terms.custom') }}
                  </Badge>
                </div>
                <p v-if="toolSecondaryLine(item)" class="text-xs text-muted-foreground">
                  {{ toolSecondaryLine(item) }}
                </p>
              </div>
              <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
                {{ humanizeDuration(item.durationMs) }}
              </span>
            </div>

            <!-- Thinking: expandable (verbose + has content) -->
            <button
              v-else-if="item.type === 'thinking' && canExpandThinking(item)"
              type="button"
              class="flex w-full items-center gap-3 rounded-md border-input bg-background px-3 py-2 text-left transition-colors hover:bg-accent/30"
              @click="toggleThinking(item.id)"
            >
              <component :is="iconFor(item)" class="h-4 w-4 shrink-0" :class="iconClassFor(item)" />
              <span class="min-w-0 flex-1 text-sm text-foreground">{{ simpleLine(item) }}</span>
              <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
                {{ humanizeDuration(item.durationMs) }}
              </span>
              <component
                :is="expandedThinkingId === item.id ? ChevronDown : ChevronRight"
                class="h-3.5 w-3.5 text-muted-foreground"
              />
            </button>

            <!-- Thinking / draft: non-expandable -->
            <div
              v-else
              class="flex items-center gap-3 rounded-md border-input bg-background px-3 py-2"
            >
              <component
                :is="iconFor(item)"
                class="h-4 w-4 shrink-0"
                :class="iconClassFor(item)"
              />
              <span class="min-w-0 flex-1 text-sm text-foreground">
                {{ simpleLine(item) }}
              </span>
              <span v-if="item.durationMs != null" class="text-xs text-muted-foreground">
                {{ humanizeDuration(item.durationMs) }}
              </span>
            </div>

            <!-- Tool expanded detail -->
            <div
              v-if="item.type === 'tool' && expandedEventId === item.id"
              class="rounded-md border border-input bg-background px-3 py-2 text-xs space-y-2"
            >
              <div v-if="item.phase !== 'completed' && item.input && Object.keys(item.input).length">
                <div class="mb-1 text-muted-foreground">{{ $t('globals.terms.inputParameters') }}</div>
                <pre class="font-mono whitespace-pre-wrap">{{ shortJson(item.input) }}</pre>
              </div>
              <div v-if="item.error" class="text-destructive">
                {{ item.error }}
              </div>
              <template v-if="item.phase === 'completed' && item.data != null">
                <KnowledgeSearchResults
                  v-if="item.toolId === 'kb_search'"
                  :results="item.data.results || []"
                />
                <div v-else>
                  <div class="mb-1 text-muted-foreground">{{ $t('globals.terms.details') }}</div>
                  <pre class="font-mono whitespace-pre-wrap">{{ shortJson(item.data) }}</pre>
                </div>
              </template>
            </div>

            <!-- Thinking expanded content (verbose) -->
            <div
              v-if="item.type === 'thinking' && canExpandThinking(item) && expandedThinkingId === item.id"
              class="rounded-md border border-input bg-background px-3 py-2 space-y-2"
            >
              <MarkdownText v-if="item.thinkingText" :source="item.thinkingText" />
              <p v-if="item.hasRedactedThinking" class="text-xs italic text-muted-foreground">
                {{ $t('composer.redactedThinking') }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleExclamation as AlertCircle, BrainCircuit, CircleCheck as CheckCircle2, ChevronDown, ChevronRight, CircleExclamation as CircleAlert, FileLines as FileText, ClockRotateLeft as History, Loader as Loader2, MagnifyingGlass as Search, WifiSlash as WifiOff, Wrench } from '@qall/icons'
import { Alert, AlertDescription, AlertTitle } from '@shared-ui/components/ui/alert'
import { Badge } from '@shared-ui/components/ui/badge'
import { Button } from '@shared-ui/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@shared-ui/components/ui/tooltip'
import MarkdownText from '@shared-ui/components/MarkdownText.vue'
import KnowledgeSearchResults from '@/features/ai/shared/KnowledgeSearchResults.vue'
import {
  buildActivityEventsFromStream,
  buildActivityEventsFromTrace,
} from '@/features/ai/shared/runActivity.js'
import { humanizeDuration, queryPreview, shortJson } from '@/features/ai/shared/toolLabels.js'

const props = defineProps({
  result: { type: Object, default: null },
  isStale: { type: Boolean, default: false },
  streamEvents: { type: Array, default: () => [] },
  streamStatus: { type: String, default: 'idle' },
  streamError: { type: String, default: '' },
  isStreaming: { type: Boolean, default: false },
  verbose: { type: Boolean, default: false },
})

const { t } = useI18n()

const showActivity = ref(false)
const expandedEventId = ref(null)
const expandedThinkingId = ref(null)

const trace = computed(() => props.result?.trace || [])
const hasDraft = computed(
  () => typeof props.result?.draft === 'string' && props.result.draft.trim() !== ''
)
const showLiveFeed = computed(() =>
  props.isStreaming ||
  ['queued', 'starting', 'running', 'disconnected', 'failed'].includes(props.streamStatus)
)

const activityItems = computed(() =>
  showLiveFeed.value
    ? buildActivityEventsFromStream(props.streamEvents, t)
    : buildActivityEventsFromTrace(trace.value, t)
)

watch(
  () => [
    props.streamStatus,
    props.result?.status || '',
    props.result?.draft || '',
    props.result?.trace?.length || 0,
  ],
  () => {
    expandedEventId.value = null
    expandedThinkingId.value = null
    if (showLiveFeed.value) {
      showActivity.value = false
    }
  }
)

const toggleEvent = (id) => {
  expandedEventId.value = expandedEventId.value === id ? null : id
}

const toggleThinking = (id) => {
  expandedThinkingId.value = expandedThinkingId.value === id ? null : id
}

const hasExpandableContent = (item) => {
  if (item.type !== 'tool') return false
  const hasInput =
    item.input && typeof item.input === 'object' && Object.keys(item.input).length > 0
  const hasError = !!item.error
  const hasData =
    item.phase === 'completed' &&
    item.data != null &&
    typeof item.data === 'object' &&
    Object.keys(item.data).length > 0
  return hasInput || hasError || hasData
}

const canExpandThinking = (item) => {
  return (
    props.verbose &&
    item.type === 'thinking' &&
    (!!item.thinkingText || !!item.hasRedactedThinking)
  )
}

const iconFor = (item) => {
  if (item.type === 'thinking') return BrainCircuit
  if (item.type === 'draft') return FileText
  if (item.toolId === 'kb_search') return Search
  return item.status === 'success' && item.phase === 'completed' ? CheckCircle2 : Wrench
}

const iconClassFor = (item) => {
  if (item.type === 'thinking') return 'text-muted-foreground'
  if (item.type === 'draft') return 'text-foreground'
  if (item.status === 'success' && item.phase === 'completed') {
    return 'text-emerald-600 dark:text-emerald-400'
  }
  if (item.status && item.status !== 'success' && item.phase === 'completed') {
    return 'text-destructive'
  }
  return 'text-muted-foreground'
}

const simpleLine = (item) => {
  if (item.type === 'draft') return t('composer.progress.draftReady')
  if (item.pending) return t('composer.progress.thinking')
  return t('composer.thoughtFor', { duration: humanizeDuration(item.durationMs || 0) })
}

const toolLine = (item) => {
  return item.label
}

const toolSecondaryLine = (item) => {
  if (item.phase === 'dispatched') {
    const preview = queryPreview(item.input)
    return preview ? `“${preview}”` : ''
  }
  if (item.phase === 'completed' && item.query) {
    return `“${queryPreview(item.input)}”`
  }
  return ''
}
</script>
