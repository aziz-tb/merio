<!--
  Inline test panel for the tool edit form. Authors fill in test inputs,
  click Run, and see the runner result (status code, kind, duration,
  output JSON, stderr on failure).

  Visible only on the edit form (slug must exist). After the F5 schema
  cleanup ai_tools holds Python custom tools only, so this panel always
  invokes the F6 sandbox.

  Form fields are auto-generated from the tool's input_parameters JSONB:
    type=STRING/TEXT  → text input
    type=INTEGER/INT  → numeric input (integer)
    type=FLOAT/NUMBER → numeric input (decimal)
    type=BOOL/BOOLEAN → checkbox
    type=DATE         → text input (date string; runner doesn't parse here)
    type=LIST/ARRAY,
    type=OBJECT, etc. → JSON editor fallback
  If input_parameters is empty or unrecognized, a single JSON editor for
  the entire parameters object is shown.
-->
<template>
  <div class="border border-input rounded-lg p-5 space-y-4 bg-muted/20">
    <h3 class="text-base font-semibold text-foreground">{{ $t('admin.ai.tool.testPanel.title') }}</h3>

    <p class="text-sm text-muted-foreground">
      {{ $t('admin.ai.tool.testPanel.help') }}
    </p>

    <!-- Auto-generated parameter form OR JSON fallback. -->
    <div v-if="hasTypedParams" class="space-y-3">
        <div v-for="def in typedParams" :key="def.key" class="space-y-1.5">
          <Label class="text-muted-foreground">
            {{ def.name || def.key }}
            <span v-if="def.required" class="text-destructive">*</span>
            <span class="text-xs text-muted-foreground/70 font-mono">({{ def.type || 'string' }})</span>
          </Label>
          <!-- Boolean -->
          <div v-if="isBoolType(def.type)" class="flex items-center gap-2">
            <Checkbox :checked="!!params[def.key]" @update:checked="(v) => (params[def.key] = v)" />
            <span class="text-sm text-muted-foreground">{{ params[def.key] ? 'true' : 'false' }}</span>
          </div>
          <!-- Integer -->
          <Input
            v-else-if="isIntType(def.type)"
            type="number"
            step="1"
            :model-value="params[def.key] ?? ''"
            @input="(e) => (params[def.key] = e.target.value === '' ? null : parseInt(e.target.value, 10))"
          />
          <!-- Float -->
          <Input
            v-else-if="isFloatType(def.type)"
            type="number"
            step="0.001"
            :model-value="params[def.key] ?? ''"
            @input="(e) => (params[def.key] = e.target.value === '' ? null : parseFloat(e.target.value))"
          />
          <!-- LIST / OBJECT → JSON editor (one-shot fallback per field) -->
          <JsonField
            v-else-if="isJsonType(def.type)"
            :model-value="paramJson(def.key)"
            @update:model-value="(v) => (paramJsonScratch[def.key] = v)"
            height-class="h-32"
            @parse-error="(e) => (paramJsonErrors[def.key] = e)"
          />
          <!-- STRING / DATE / unknown — text input -->
          <Input
            v-else
            type="text"
            :model-value="params[def.key] ?? ''"
            @input="(e) => (params[def.key] = e.target.value)"
          />
          <p v-if="def.description" class="text-xs text-muted-foreground">{{ def.description }}</p>
        </div>
      </div>

      <!-- No declared input_parameters → single JSON editor for the whole map. -->
      <div v-else class="space-y-2">
        <Label class="text-muted-foreground">{{ $t('admin.ai.tool.testPanel.parametersJson') }}</Label>
        <JsonField v-model="rawParamsJson" height-class="h-40" @parse-error="rawParamsParseError = $event" />
      </div>

      <!-- Optional conversation_id -->
      <div class="space-y-1.5">
        <Label class="text-muted-foreground">{{ $t('admin.ai.tool.testPanel.conversationId') }}</Label>
        <Input
          type="number"
          step="1"
          min="0"
          :model-value="conversationId"
          @input="(e) => (conversationId = e.target.value === '' ? 0 : parseInt(e.target.value, 10))"
          placeholder="0"
        />
        <p class="text-xs text-muted-foreground">
          {{ $t('admin.ai.tool.testPanel.conversationHelp') }}
        </p>
      </div>

    <Button
      @click="runTool"
      :is-loading="isRunning"
      :disabled="isRunning || !canRun"
      class="w-full"
    >
      {{ $t('admin.ai.tool.testPanel.run') }}
    </Button>

    <!-- Result / Error display -->
    <div v-if="lastResult" class="space-y-3 pt-2 border-t border-input">
      <div class="flex flex-wrap items-center gap-2">
        <Badge :variant="lastResult.httpOk ? 'default' : 'destructive'">
          {{ lastResult.httpStatus }}
        </Badge>
        <Badge v-if="lastResult.kind" variant="secondary" class="font-mono font-normal">
          {{ lastResult.kind }}
        </Badge>
        <Badge v-if="lastResult.code" variant="outline" class="font-mono font-normal text-xs">
          {{ lastResult.code }}
        </Badge>
        <Badge v-if="lastResult.duration_ms != null" variant="secondary" class="font-mono font-normal">
          {{ lastResult.duration_ms }}ms
        </Badge>
      </div>
      <div v-if="lastResult.execution_id" class="text-xs font-mono text-muted-foreground">
        {{ $t('admin.ai.tool.testPanel.executionId') }}: {{ lastResult.execution_id }}
      </div>

      <div v-if="lastResult.message" class="text-sm">
        <span class="text-muted-foreground">{{ $t('admin.ai.tool.testPanel.message') }}:</span>
        <span class="ml-2">{{ lastResult.message }}</span>
      </div>

      <div v-if="lastResult.outputJson">
        <CodeEditor
          :model-value="lastResult.outputJson"
          language="json"
          read-only
          :title="$t('admin.ai.tool.testPanel.output')"
          show-language-label
          expandable
          height-class="h-48"
        />
      </div>

      <div v-if="lastResult.stderr">
        <CodeEditor
          :model-value="lastResult.stderr"
          language="text"
          read-only
          :title="$t('admin.ai.tool.testPanel.stderr')"
          expandable
          height-class="h-48"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import CodeEditor from '@/components/editor/CodeEditor.vue'
import JsonField from '@/features/ai/shared/JsonField.vue'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  tool: { type: Object, required: true },
  // The tool's parsed input_parameters (array of {key, name, type, required, description}).
  // Empty/null array means we render the JSON-editor fallback.
  inputParameters: { type: Array, default: () => [] }
})

const { t } = useI18n()
const emitter = useEmitter()

const isRunning = ref(false)
const lastResult = ref(null)

// Reactive parameter map driven by the auto-generated form.
const params = reactive({})

// Per-field JSON editor state for LIST/OBJECT fields. paramJsonScratch
// holds the in-flight string so the user can type freely; paramJsonErrors
// captures parse errors per field; paramJson() is the read accessor.
const paramJsonScratch = reactive({})
const paramJsonErrors = reactive({})
function paramJson(key) {
  if (key in paramJsonScratch) return paramJsonScratch[key]
  if (params[key] !== undefined && params[key] !== null) return JSON.stringify(params[key], null, 2)
  return '[]'
}

// Fallback whole-object JSON editor (used when no typed params declared).
const rawParamsJson = ref('{}')
const rawParamsParseError = ref('')

const conversationId = ref(0)

const typedParams = computed(() => Array.isArray(props.inputParameters) ? props.inputParameters : [])
const hasTypedParams = computed(() => typedParams.value.length > 0)

// canRun is true when no parse errors are pending across the JSON fields.
const canRun = computed(() => {
  if (!hasTypedParams.value) return !rawParamsParseError.value
  return !Object.values(paramJsonErrors).some(Boolean)
})

const isBoolType = (t) => /^(BOOL|BOOLEAN)$/i.test(String(t || ''))
const isIntType = (t) => /^(INTEGER|INT)$/i.test(String(t || ''))
const isFloatType = (t) => /^(FLOAT|NUMBER)$/i.test(String(t || ''))
const isJsonType = (t) => /^(LIST|ARRAY|OBJECT|MAP|JSON)$/i.test(String(t || ''))

function buildParameters() {
  if (!hasTypedParams.value) {
    if (!rawParamsJson.value.trim()) return {}
    return JSON.parse(rawParamsJson.value)
  }
  const out = {}
  for (const def of typedParams.value) {
    if (isJsonType(def.type)) {
      const raw = paramJsonScratch[def.key]
      if (raw !== undefined && raw.trim() !== '') {
        out[def.key] = JSON.parse(raw)
      } else if (params[def.key] !== undefined) {
        out[def.key] = params[def.key]
      }
    } else if (params[def.key] !== undefined && params[def.key] !== null && params[def.key] !== '') {
      out[def.key] = params[def.key]
    }
  }
  return out
}

async function runTool() {
  if (props.tool.type === 'go_builtin') return
  isRunning.value = true
  lastResult.value = null
  let payload
  try {
    payload = {
      parameters: buildParameters(),
      conversation_id: conversationId.value || 0
    }
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: `${t('admin.ai.invalidJson')}: ${e.message}`
    })
    isRunning.value = false
    return
  }
  try {
    const resp = await api.invokeAITool(props.tool.slug, payload)
    lastResult.value = decodeResult(resp.status, resp.data, true)
  } catch (err) {
    // Axios surfaces non-2xx as an exception; we want the structured body.
    if (err && err.response) {
      lastResult.value = decodeResult(err.response.status, err.response.data, false)
    } else {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(err).message
      })
    }
  } finally {
    isRunning.value = false
  }
}

// decodeResult normalizes both success-envelope and error-envelope responses
// into a single shape for the result display block.
function decodeResult(httpStatus, body, httpOk) {
  // The endpoint envelopes both success and error responses with a `data`
  // field. On success, data is the runner's Result. On error, data is also
  // the Result (200-bodied even though status is 4xx/5xx, to surface stderr
  // and execution telemetry).
  const r = body && body.data
  let outputJson = ''
  if (r && r.data && Object.keys(r.data).length > 0) {
    // Strip the operational _stderr key from output JSON — we display it
    // separately in the stderr block.
    const out = { ...r.data }
    delete out._stderr
    outputJson = JSON.stringify(out, null, 2)
  } else if (httpOk && r) {
    outputJson = JSON.stringify(r, null, 2)
  }
  return {
    httpStatus,
    httpOk,
    kind: r?.kind ?? '',
    code: r?.code ?? '',
    message: r?.message ?? body?.message ?? '',
    duration_ms: r?.duration_ms ?? null,
    execution_id: r?.execution_id ?? '',
    outputJson,
    stderr: r?.stderr ?? r?.data?._stderr ?? ''
  }
}
</script>
