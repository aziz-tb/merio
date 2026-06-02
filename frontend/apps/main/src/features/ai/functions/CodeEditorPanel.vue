<template>
  <div class="space-y-4">
    <div :class="['grid gap-5', showsTimeout ? 'md:grid-cols-3' : 'md:grid-cols-2']">
      <div class="space-y-2">
        <Label class="text-muted-foreground">{{ t('admin.ai.function.language') }}</Label>
        <Select v-model="languageModel">
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
        <Input v-model="packagesModel" @keydown.enter.prevent />
        <p class="text-xs text-muted-foreground">
          {{ t('admin.ai.function.packagesHelp') }}
        </p>
      </div>

      <div v-if="showsTimeout" class="space-y-2">
        <Label class="text-muted-foreground">{{ t('admin.ai.function.timeout') }}</Label>
        <Input
          type="number"
          min="1"
          max="120000"
          step="1000"
          :model-value="timeoutMsModel"
          @input="(e) => setTimeoutMs(e.target.value)"
        />
        <p class="text-xs text-muted-foreground">
          {{ t('admin.ai.function.timeoutHelp') }}
        </p>
      </div>
    </div>

    <CodeEditor
      :key="languageModel"
      v-model="codeModel"
      :language="languageModel"
      :height-class="heightClass"
      show-language-label
      expandable
    />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import CodeEditor from '@/components/editor/CodeEditor.vue'

const props = defineProps({
  code: { type: String, default: '' },
  language: { type: String, default: 'python' },
  packages: { type: String, default: '' },
  timeoutMs: { type: Number, default: null },
  title: { type: String, default: '' },
  heightClass: { type: String, default: 'h-[28rem]' }
})

const emit = defineEmits([
  'update:code',
  'update:language',
  'update:packages',
  'update:timeoutMs'
])

const { t } = useI18n()

const codeModel = computed({
  get: () => props.code || '',
  set: (value) => emit('update:code', value)
})

const languageModel = computed({
  get: () => props.language || 'python',
  set: (value) => emit('update:language', value)
})

const packagesModel = computed({
  get: () => props.packages || '',
  set: (value) => emit('update:packages', value)
})

const showsTimeout = computed(() => props.timeoutMs !== null && props.timeoutMs !== undefined)
const timeoutMsModel = computed(() => Number(props.timeoutMs || 30000))

function setTimeoutMs(value) {
  const next = Number(value)
  emit('update:timeoutMs', Number.isFinite(next) ? next : 1)
}
</script>
