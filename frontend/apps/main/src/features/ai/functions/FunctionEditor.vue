<template>
  <LoadingOverlay :loading="loading" reserve-height>
    <div class="bg-background p-5 space-y-5">
      <Alert v-if="loadError" variant="destructive">
        <AlertDescription>{{ loadError }}</AlertDescription>
      </Alert>

      <CodeEditorPanel
        v-model:code="functionRow.code"
        v-model:language="functionRow.language"
        v-model:packages="functionRow.packages"
        :timeout-ms="functionRow.timeout_ms"
        @update:timeout-ms="setTimeoutMs"
      />

     <FunctionPreviewPanel
      :name="props.name"
      :id="functionRow.id"
      :code="functionRow.code"
      :language="functionRow.language"
      :packages="functionRow.packages"
      v-model:parameters="previewParameters"
    >
      <template #actions>
        <Button type="button" :is-loading="saving" @click="save">
          <FloppyDisk class="size-4" />
          {{ saving ? t('admin.ai.function.saving') : t('globals.messages.save') }}
        </Button>
      </template>
    </FunctionPreviewPanel>
    
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FloppyDisk } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button'
import {
  Alert,
  AlertDescription
} from '@shared-ui/components/ui/alert'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import CodeEditorPanel from '@/features/ai/functions/CodeEditorPanel.vue'
import FunctionPreviewPanel from '@/features/ai/functions/FunctionPreviewPanel.vue'
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
const loadError = ref('')

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
