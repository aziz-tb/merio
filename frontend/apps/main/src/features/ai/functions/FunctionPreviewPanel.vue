<template>
  <div class="space-y-5">
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
            <TableRow
              v-for="param in rows"
              :key="param.id"
              class="border-background hover:bg-background"
            >
              <TableCell>
                <Input v-model="param.key" placeholder="customerId" @input="emitRows" @keydown.enter.prevent />
              </TableCell>
              <TableCell>
                <Input v-model="param.value" placeholder="12345" @input="emitRows" @keydown.enter.prevent />
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
            <TableRow v-if="rows.length === 0">
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
      <Tooltip v-if="!canPreview">
        <TooltipTrigger as-child>
          <span class="inline-flex">
            <Button type="button" variant="secondary" disabled>
              <Play class="size-4" />
              {{ t('admin.ai.function.preview') }}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent side="top">
          <p>{{ t('admin.ai.function.previewRequiresName') }}</p>
        </TooltipContent>
      </Tooltip>

      <Button
        v-else
        type="button"
        variant="secondary"
        :disabled="previewDisabled"
        :is-loading="previewing"
        @click="preview"
      >
        <Play class="size-4" />
        {{ previewing ? t('admin.ai.function.previewing') : t('admin.ai.function.preview') }}
      </Button>

      <slot name="actions" />
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
        <div class="p-3">
          <JsonTreeViewer
            v-if="hasPreviewTreeResults"
            :value="previewDataTree"
            :searchable="false"
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
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { TrashCan, Play, Plus, X } from '@qall/icons'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button'
import { Badge } from '@shared-ui/components/ui/badge'
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger
} from '@shared-ui/components/ui/tooltip'
import JsonTreeViewer from '@/features/admin/detection/JsonTreeViewer.vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  name: { type: String, required: true },
  code: { type: String, default: '' },
  language: { type: String, default: 'python' },
  packages: { type: String, default: '' },
  id: { type: Number, default: 0 },
  parameters: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:parameters'])

const { t } = useI18n()

const rows = ref(normalizeParameters(props.parameters))
const previewing = ref(false)
const previewResult = ref(null)
const previewError = ref('')
const runtimeError = ref(null)
const runtimeErrorDetailsOpen = ref(false)
const dataSearch = ref('')

const canPreview = computed(() => String(props.name || '').trim() !== '')
const previewDisabled = computed(() => previewing.value || !canPreview.value)

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
      return 'border-green-600 bg-transparent text-green-600'
    case 'failure':
      return 'border-destructive-foreground bg-transparent text-destructive-foreground'
    case 'error':
      return 'border-orange-600 bg-transparent text-orange-600'
    case 'noop':
      return 'border-muted-foreground text-transparent'
    default:
      return ''
  }
})

watch(
  () => props.parameters,
  (incoming) => {
    if (incoming === rows.value) return
    rows.value = normalizeParameters(incoming)
  },
  { deep: true }
)

function normalizeParameters(list) {
  return (list || []).map(newParam)
}

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

function emitRows() {
  emit('update:parameters', rows.value.map((row) => ({ ...row })))
}

function addParameter() {
  rows.value.push(newParam())
  emitRows()
}

function removeParameter(id) {
  rows.value = rows.value.filter((p) => p.id !== id)
  emitRows()
}

function clearPreview() {
  previewResult.value = null
  previewError.value = ''
  runtimeError.value = null
  runtimeErrorDetailsOpen.value = false
  dataSearch.value = ''
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
      id: props.id || 0,
      code: props.code || '',
      language: props.language || 'python',
      packages: props.packages || '',
      parameters: rows.value
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
</script>
