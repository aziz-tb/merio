<template>
  <LoadingOverlay :loading="loading" reserve-height>
    <div class="bg-background p-5 space-y-5">

      <Alert v-if="loadError" variant="destructive">
        <AlertDescription>{{ loadError }}</AlertDescription>
      </Alert>

      <div class="grid gap-5 md:grid-cols-3">
        <div class="space-y-2">
          <Label class="text-muted-foreground">{{ t('admin.ai.function.language') }}</Label>
          <Select v-model="functionRow.language">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="python">Python</SelectItem>
              <SelectItem value="javascript">JavaScript</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="space-y-2">
          <Label class="text-muted-foreground">{{ t('admin.ai.function.packages') }}</Label>
          <Input v-model="functionRow.packages" @keydown.enter.prevent />
          <p class="text-xs text-muted-foreground">
            {{ t('admin.ai.function.packagesHelp') }}
          </p>
        </div>

        <div class="space-y-2">
          <Label class="text-muted-foreground">{{ t('admin.ai.function.timeout') }}</Label>
          <Input
            type="number"
            min="1"
            max="120000"
            step="1000"
            :model-value="functionRow.timeout_ms"
            @input="(e) => setTimeoutMs(e.target.value)"
          />
          <p class="text-xs text-muted-foreground">
            {{ t('admin.ai.function.timeoutHelp') }}
          </p>
        </div>
      </div>

      <CodeEditor
        :key="functionRow.language"
        v-model="functionRow.code"
        :language="functionRow.language"
        :title="t('globals.terms.implementationCode')"
        height-class="h-[28rem]"
        show-language-label
        expandable
      />


      <div class="space-y-3">
        <div>
          <h4 class="text-sm font-medium text-foreground">
            {{ t('admin.ai.function.previewParameters') }}
          </h4>
          <p class="text-xs text-muted-foreground">
            {{ t('admin.ai.function.previewParametersHelp') }}
          </p>
        </div>

        <div class="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow class="border-background hover:bg-background">
                <TableHead class="w-[38%]">
                  {{ t('admin.ai.tool.params.key') }}
                </TableHead>
                <TableHead>{{ t('admin.ai.function.parameterValue') }}</TableHead>
                <TableHead class="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="param in previewParameters" :key="param.id" class="border-background hover:bg-background">
                <TableCell>
                  <Input v-model="param.key" placeholder="customerId" @keydown.enter.prevent />
                </TableCell>
                <TableCell>
                  <Input v-model="param.value" placeholder="12345" @keydown.enter.prevent />
                </TableCell>
                <TableCell class="text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    :title="t('globals.messages.remove')"
                    @click="removeParameter(param.id)"
                  >
                    <TrashCan class="size-4" />
                    <span class="sr-only">{{ t('globals.messages.remove') }}</span>
                  </Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="previewParameters.length === 0">
                <TableCell colspan="3" class="text-sm text-muted-foreground">
                  {{ t('admin.ai.tool.params.empty') }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <Button type="button" variant="outline" size="sm" class="ml-2" @click="addParameter">
          <Plus class="size-4" />
          {{ t('admin.ai.function.addParameter') }}
        </Button>
      </div>

      <div class="flex justify-end gap-2">
        <Button
          type="button"
          variant="secondary"
          :disabled="previewDisabled"
          :is-loading="previewing"
          @click="preview"
        >
          <Play class="size-4" />
          {{ previewing ? t('admin.ai.function.previewing') : t('admin.ai.function.preview') }}
        </Button>

        <Button type="button" :is-loading="saving" @click="save">
          <FloppyDisk class="size-4" />
          {{ saving ? t('admin.ai.function.saving') : t('globals.messages.save') }}
        </Button>
      </div>

      <Alert v-if="previewError" variant="destructive">
        <AlertDescription>{{ previewError }}</AlertDescription>
      </Alert>

      <div v-if="runtimeError" class="space-y-3 rounded-lg border border-destructive/40 bg-destructive/5 p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="space-y-2">
            <h4 class="text-sm font-semibold text-destructive">
              {{ t('admin.ai.function.runtimeError') }}
            </h4>
            <div class="flex flex-wrap items-center gap-2">
              <Badge variant="destructive">
                {{ runtimeError.kind || 'runtime_error' }}
              </Badge>
              <Badge v-if="runtimeError.code" variant="outline" class="font-mono font-normal">
                {{ runtimeError.code }}
              </Badge>
            </div>
          </div>
          <Button type="button" variant="ghost" size="sm" @click="clearPreview">
            <X class="size-4" />
            {{ t('admin.ai.function.clearPreview') }}
          </Button>
        </div>

        <p v-if="runtimeError.message" class="text-sm text-foreground whitespace-pre-wrap">
          {{ runtimeError.message }}
        </p>

        <div v-if="runtimeError.traceback" class="space-y-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            @click="runtimeErrorDetailsOpen = !runtimeErrorDetailsOpen"
          >
            {{ runtimeErrorDetailsOpen ? t('admin.ai.function.hideDetails') : t('admin.ai.function.showDetails') }}
          </Button>
          <pre
            v-if="runtimeErrorDetailsOpen"
            class="max-h-80 overflow-auto rounded-md bg-muted/40 p-3 text-xs font-mono whitespace-pre-wrap"
          >{{ runtimeError.traceback }}</pre>
        </div>
      </div>

      <div v-if="previewResult" class="space-y-4 rounded-lg border border-input p-4">
        <div class="flex items-start justify-between gap-3">
          <h4 class="text-sm font-semibold text-foreground">
            {{ t('admin.ai.function.result') }}
          </h4>
          <Button type="button" variant="ghost" size="sm" @click="clearPreview">
            <X class="size-4" />
            {{ t('admin.ai.function.clearPreview') }}
          </Button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <Badge :class="resultKindClass">
            {{ previewResult.kind || 'unknown' }}
          </Badge>
          <Badge v-if="previewResult.code" variant="outline" class="font-mono font-normal">
            {{ previewResult.code }}
          </Badge>
          <Badge v-if="previewResult.duration_ms != null" variant="secondary" class="font-mono font-normal">
            {{ previewResult.duration_ms }} ms
          </Badge>
        </div>

        <div v-if="previewResult.message" class="space-y-1">
          <p class="text-xs font-medium text-muted-foreground">
            {{ t('admin.ai.function.resultMessage') }}
          </p>
          <p class="text-sm text-foreground">{{ previewResult.message }}</p>
        </div>

        <div v-if="previewDataJson" class="space-y-1">
          <p class="text-xs font-medium text-muted-foreground">
            {{ t('admin.ai.function.resultData') }}
          </p>
          <Input
            v-model="dataSearch"
            :placeholder="t('admin.ai.function.searchData')"
            class="max-w-sm"
          />
          <div class="max-h-80 overflow-auto rounded-md bg-muted/40 p-3">
            <JsonTreeViewer
              v-if="hasPreviewTreeResults"
              :value="previewDataTree"
            />
            <p v-else class="text-sm text-muted-foreground">
              {{ t('globals.messages.noResultsFound') }}
            </p>
          </div>
        </div>

        <div v-if="previewResult.stderr" class="space-y-1">
          <p class="text-xs font-medium text-muted-foreground">
            {{ t('admin.ai.function.resultStderr') }}
          </p>
          <pre class="max-h-80 overflow-auto rounded-md bg-muted/40 p-3 text-xs font-mono whitespace-pre-wrap">{{ previewResult.stderr }}</pre>
        </div>
      </div>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FloppyDisk, TrashCan, Play, Plus, X } from '@qall/icons'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@shared-ui/components/ui/table'
import {
  Alert,
  AlertDescription
} from '@shared-ui/components/ui/alert'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import CodeEditor from '@/components/editor/CodeEditor.vue'
import JsonTreeViewer from '@/features/admin/detection/JsonTreeViewer.vue'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  name: { type: String, required: true }
})

const { t } = useI18n()
const emitter = useEmitter()

const defaultFunction = () => ({
  id: 0,
  name: props.name,
  domain: '',
  code: '',
  language: 'python',
  packages: '',
  timeout_ms: 30000,
  parameters: []
})

const functionRow = ref(defaultFunction())
const previewParameters = ref([])
const loading = ref(false)
const saving = ref(false)
const previewing = ref(false)
const previewResult = ref(null)
const previewError = ref('')
const runtimeError = ref(null)
const runtimeErrorDetailsOpen = ref(false)
const loadError = ref('')
const dataSearch = ref('')

const previewDisabled = computed(() =>
  loading.value || previewing.value || saving.value
)

const previewDataJson = computed(() =>
  previewResult.value?.data == null ? '' : JSON.stringify(previewResult.value.data, null, 2)
)

const previewDataTree = computed(() => {
  const data = previewResult.value?.data
  if (data == null) return null
  const query = dataSearch.value.trim().toLowerCase()
  if (!query) return normalizeTreeRoot(data)
  return normalizeTreeRoot(filterTree(data, query))
})

const hasPreviewTreeResults = computed(() => !isEmptyTree(previewDataTree.value))

const resultKindClass = computed(() => {
  switch (previewResult.value?.kind) {
    case 'success':
      return 'border-transparent bg-green-600 text-white hover:bg-green-600'
    case 'failure':
      return 'border-transparent bg-destructive text-destructive-foreground hover:bg-destructive'
    case 'error':
      return 'border-transparent bg-orange-600 text-white hover:bg-orange-600'
    case 'noop':
      return 'border-transparent bg-muted text-muted-foreground hover:bg-muted'
    default:
      return ''
  }
})

function newParam(row = {}) {
  return {
    id: row.id || makeID(),
    key: row.key || '',
    value: row.value || ''
  }
}

function makeID() {
  if (window.crypto?.randomUUID) return window.crypto.randomUUID()
  return `param-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function parseParameters(raw) {
  if (Array.isArray(raw)) return raw.map(newParam)
  if (typeof raw === 'string' && raw.trim()) {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed.map(newParam) : []
    } catch {
      return []
    }
  }
  return []
}

function normalizeFunction(row) {
  const language = String(row?.language || 'python').toLowerCase()
  return {
    ...defaultFunction(),
    ...row,
    language: language === 'javascript' ? 'javascript' : 'python',
    packages: row?.packages || '',
    timeout_ms: Number(row?.timeout_ms || 30000)
  }
}

function setTimeoutMs(value) {
  const next = Number(value)
  functionRow.value.timeout_ms = Number.isFinite(next) ? next : 1
}

function addParameter() {
  previewParameters.value.push(newParam())
}

function removeParameter(id) {
  previewParameters.value = previewParameters.value.filter((p) => p.id !== id)
}

function clearPreview() {
  previewResult.value = null
  previewError.value = ''
  runtimeError.value = null
  runtimeErrorDetailsOpen.value = false
  dataSearch.value = ''
}

function savePayload() {
  return {
    code: functionRow.value.code || '',
    language: functionRow.value.language || 'python',
    packages: functionRow.value.packages || '',
    timeout_ms: Number(functionRow.value.timeout_ms || 30000),
    parameters: previewParameters.value
  }
}

async function loadFunction() {
  loading.value = true
  loadError.value = ''
  previewResult.value = null
  previewError.value = ''
  runtimeError.value = null
  runtimeErrorDetailsOpen.value = false
  try {
    const resp = await api.getFunction(props.name)
    const row = resp.data.data
    functionRow.value = normalizeFunction(row)
    previewParameters.value = parseParameters(row?.parameters)
  } catch (error) {
    loadError.value = handleHTTPError(error).message
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: loadError.value
    })
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const resp = await api.updateFunction(props.name, savePayload())
    const row = resp.data.data
    functionRow.value = normalizeFunction(row)
    previewParameters.value = parseParameters(row?.parameters)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.ai.function.saveSuccess')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    saving.value = false
  }
}

async function preview() {
  if (previewDisabled.value) return
  previewing.value = true
  previewError.value = ''
  previewResult.value = null
  runtimeError.value = null
  runtimeErrorDetailsOpen.value = false
  dataSearch.value = ''
  try {
    const body = {
      id: functionRow.value.id,
      code: functionRow.value.code || '',
      language: functionRow.value.language || 'python',
      packages: functionRow.value.packages || '',
      parameters: previewParameters.value
    }
    const resp = await api.previewFunction(props.name, body)
    previewResult.value = resp.data.data
  } catch (error) {
    const httpError = handleHTTPError(error)
    const rt = runtimeErrorFromHTTP(httpError, error)
    if (rt) {
      runtimeError.value = rt
      return
    }
    previewError.value = t('admin.ai.function.previewError', {
      error: httpError.message
    })
  } finally {
    previewing.value = false
  }
}

function runtimeErrorFromHTTP(httpError, rawError) {
  const data = httpError?.data || rawError?.response?.data?.data || rawError?.response?.data
  if (!data || data.kind !== 'runtime_error') return null
  return {
    kind: data.kind,
    code: data.code || 'UNCAUGHT',
    message: data.message || httpError?.message || '',
    traceback: data.traceback || ''
  }
}

function normalizeTreeRoot(value) {
  if (value === undefined) return null
  if (value !== null && typeof value === 'object') return value
  return { value }
}

function filterTree(value, query, key = '') {
  const keyMatches = key.toLowerCase().includes(query)
  if (value !== null && typeof value === 'object') {
    if (keyMatches) return value
    if (Array.isArray(value)) {
      const filtered = value
        .map((item, index) => filterTree(item, query, String(index)))
        .filter((item) => item !== undefined)
      return filtered.length > 0 ? filtered : undefined
    }
    const filtered = {}
    for (const [childKey, childValue] of Object.entries(value)) {
      const child = filterTree(childValue, query, childKey)
      if (child !== undefined) {
        filtered[childKey] = child
      }
    }
    return Object.keys(filtered).length > 0 ? filtered : undefined
  }
  const valueMatches = String(value ?? '').toLowerCase().includes(query)
  return keyMatches || valueMatches ? value : undefined
}

function isEmptyTree(value) {
  if (value == null) return true
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value).length === 0
  return false
}

onMounted(loadFunction)

watch(
  () => props.name,
  () => {
    functionRow.value = defaultFunction()
    previewParameters.value = []
    loadFunction()
  }
)
</script>
