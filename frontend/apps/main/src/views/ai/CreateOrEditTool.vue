<template>
  <div class="flex flex-col h-full overflow-auto px-6 py-4">
    <div class="mb-5">
      <CustomBreadcrumb :links="breadcrumbLinks" />
    </div>
    <LoadingOverlay :loading="isLoading">
      <form @submit="onSubmit" class="space-y-5">
      <FormField
        v-slot="{ value, handleChange }"
        type="checkbox"
        name="enabled"
        v-if="!isNewForm"
      >
        <FormItem class="flex flex-row items-start gap-x-3 space-y-0">
          <FormControl>
            <Checkbox :checked="value" @update:checked="handleChange" />
          </FormControl>
          <div class="space-y-1 leading-none">
            <FormLabel class="text-foreground">{{ $t('globals.terms.enabled') }}</FormLabel>
            <FormMessage />
          </div>
        </FormItem>
      </FormField>

      <FormField v-slot="{ field }" name="name">
        <FormItem>
          <FormLabel>{{ $t('globals.terms.name') }}</FormLabel>
          <FormControl>
            <Input type="text" v-bind="field" @keydown.enter.prevent />
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>

      <!-- Slug is auto-derived on create, locked on edit. The dispatcher
           resolves tools by slug, so changing it would orphan agents that
           reference this tool. Backend rejects PUTs with mismatched slug. -->
      <FormItem>
        <Label class="text-muted-foreground">{{ $t('globals.terms.slug') }}</Label>
        <Input
          type="text"
          :model-value="isNewForm ? autoSlugPreview : (form.values.slug || '')"
          readonly
          tabindex="-1"
          class="cursor-not-allowed bg-muted/40"
        />
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.slugPermanent') }}</p>
      </FormItem>

      <FormField v-slot="{ field }" name="description">
        <FormItem>
          <FormLabel>{{ $t('globals.terms.description') }}</FormLabel>
          <FormControl>
            <Input type="text" v-bind="field" @keydown.enter.prevent />
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>

      <div class="grid grid-cols-2 gap-5">
        <FormField v-slot="{ field }" name="timeout_ms">
          <FormItem>
            <FormLabel>{{ $t('globals.terms.timeoutMs') || 'Timeout (ms)' }}</FormLabel>
            <FormControl>
              <Input
                type="number"
                step="1000"
                min="1"
                max="600000"
                v-bind="field"
                @input="(e) => form.setFieldValue('timeout_ms', Number(e.target.value))"
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        </FormField>

        <FormField v-slot="{ componentField }" name="category">
          <FormItem>
            <FormLabel>{{ $t('globals.terms.category') || 'Category' }}</FormLabel>
            <FormControl>
              <Select v-bind="componentField">
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="read">Read (parallel)</SelectItem>
                  <SelectItem value="write">Write (sequential)</SelectItem>
                </SelectContent>
              </Select>
            </FormControl>
            <FormDescription>
              Reads can run in parallel within a single agent turn. Writes run sequentially after all reads complete. Use "write" for any tool that mutates state.
            </FormDescription>
            <FormMessage />
          </FormItem>
        </FormField>
      </div>

      <FormField v-slot="{ value, handleChange }" type="checkbox" name="async">
        <FormItem class="flex flex-row items-start gap-x-3 space-y-0">
          <FormControl>
            <Checkbox :checked="value" @update:checked="handleChange" />
          </FormControl>
          <div class="space-y-1 leading-none">
            <FormLabel class="text-foreground">{{ $t('globals.terms.async') || 'Async' }}</FormLabel>
            <FormMessage />
          </div>
        </FormItem>
      </FormField>

      <div class="space-y-2">
        <Label class="text-muted-foreground">{{ $t('globals.terms.inputParameters') }}</Label>
        <ToolParametersTable v-model="parametersRows" />
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.tool.inputParametersHelp') }}</p>
      </div>

      <div class="space-y-2">
        <JsonField
          v-model="outputSchemaJson"
          height-class="h-40"
          :title="$t('globals.terms.outputSchema')"
          show-language-label
          expandable
          @parse-error="outputParseError = $event"
        />
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.tool.outputSchemaHelp') }}</p>
      </div>

      <!-- Implementation editor. ai_tools dispatches to the F6 exec runner.
           Python tools have the libredesk SDK preamble injected; JavaScript
           runs without SDK (v1) — admins compose stateless transforms there.
           Implementation fields are local refs (not vee-validate state) so
           they use plain Label. -->
      <div class="space-y-4">
        <div class="space-y-2">
          <CodeEditor
            v-model="pythonCode"
            :language="implLanguage"
            :title="$t('globals.terms.implementationCode')"
            show-language-label
            expandable
            height-class="h-[36rem]"
          />
          <p class="text-sm text-muted-foreground">
            {{
              implLanguage === 'javascript'
                ? $t('admin.ai.tool.implementationJsHelp')
                : $t('admin.ai.tool.implementationPythonHelp')
            }}
          </p>
        </div>
        <div class="grid grid-cols-2 gap-5">
          <div class="space-y-2">
            <Label class="text-muted-foreground flex items-center gap-2">
              {{ $t('globals.terms.language') }}
              <Badge v-if="implLanguage === 'javascript'" variant="secondary">
                {{ $t('admin.ai.tool.noSdkBadge') }}
              </Badge>
            </Label>
            <Select v-model="implLanguage">
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
            <Label class="text-muted-foreground">{{ $t('globals.terms.packages') }}</Label>
            <Input
              v-model="pythonPackages"
              :disabled="implLanguage === 'javascript'"
              :placeholder="implLanguage === 'javascript' ? $t('admin.ai.tool.packagesJsDisabled') : ''"
              @keydown.enter.prevent
            />
          </div>
        </div>
      </div>

        <Button type="submit" :isLoading="isLoading">
          {{ isNewForm ? $t('globals.messages.create') : $t('globals.messages.save') }}
        </Button>
      </form>

      <!-- Inline test panel — only on edit (slug must exist for invoke).
           Renders for both python_script (Run enabled) and go_builtin
           (Run disabled with explanatory tooltip text). -->
      <ToolTestPanel
        v-if="!isNewForm && form.values.slug"
        class="mt-8"
        :tool="{
          slug: form.values.slug,
          type: form.values.type
        }"
        :input-parameters="parsedInputParameters"
      />
    </LoadingOverlay>
  </div>
</template>

<script setup>
import { onMounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Button } from '@shared-ui/components/ui/button'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Badge } from '@shared-ui/components/ui/badge'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@shared-ui/components/ui/select'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription
} from '@shared-ui/components/ui/form'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import { CustomBreadcrumb } from '@shared-ui/components/ui/breadcrumb'
import CodeEditor from '@/components/editor/CodeEditor.vue'
import JsonField from '@/features/ai/shared/JsonField.vue'
import ToolTestPanel from '@/features/ai/tools/testing/ToolTestPanel.vue'
import ToolParametersTable from '@/features/ai/tools/form/ToolParametersTable.vue'
import { slugify } from '@/utils/slugify.js'
import { createToolFormSchema } from '@/features/ai/tools/form/formSchema.js'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({ id: { type: [String, Number], required: false } })

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()

const isLoading = ref(false)

// input_parameters is now edited via the typed ToolParametersTable;
// the rows array is the canonical state. output_schema keeps the JSON
// editor (it's a generic JSON Schema fragment — typing it would
// duplicate work the JSON editor already handles).
const parametersRows = ref([])
const outputSchemaJson = ref('{}')
const outputParseError = ref('')

// Implementation fields. ai_tools dispatches to the F6 exec runner; the
// backend locks language to "python" or "javascript" at save time. JS
// runs without SDK in v1 — the form surfaces a badge + alt help text.
const pythonCode = ref('')
const implLanguage = ref('python')
const pythonPackages = ref('')

const form = useForm({
  validationSchema: toTypedSchema(createToolFormSchema(t)),
  initialValues: {
    name: '',
    slug: '',
    description: '',
    category: 'write',
    timeout_ms: 30000,
    async: false,
    enabled: true
  }
})

const isNewForm = computed(() => !props.id)

const autoSlugPreview = computed(() => slugify(form.values.name || ''))

// Parsed input_parameters for the test panel — the typed-table rows
// are already the canonical schema shape so no JSON parsing needed.
const parsedInputParameters = computed(() =>
  (parametersRows.value || []).filter((r) => r && r.key)
)

const breadcrumbLinks = computed(() => [
  { path: 'ai-tool-list', label: t('globals.terms.aiTool', 2) },
  { path: '', label: props.id ? t('aiTool.edit') : t('aiTool.new') }
])

const onSubmit = form.handleSubmit(async (values) => {
  if (outputParseError.value) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('admin.ai.invalidJson')
    })
    return
  }

  let payload
  try {
    payload = {
      ...values,
      input_parameters: (parametersRows.value || []).filter((r) => r && r.key),
      output_schema: outputSchemaJson.value.trim() ? JSON.parse(outputSchemaJson.value) : {},
      implementation: {
        code: pythonCode.value,
        language: implLanguage.value,
        packages: implLanguage.value === 'javascript' ? '' : pythonPackages.value
      }
    }
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: `${t('admin.ai.invalidJson')}: ${e.message}`
    })
    return
  }

  try {
    isLoading.value = true
    if (props.id) {
      await api.updateAITool(props.id, payload)
    } else {
      await api.createAITool(payload)
      router.push({ name: 'ai-tool-list' })
    }
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
})

onMounted(async () => {
  if (props.id) {
    try {
      isLoading.value = true
      const resp = await api.getAITool(props.id)
      const tool = resp.data.data
      form.setValues({
        name: tool.name,
        slug: tool.slug,
        description: tool.description || '',
        category: tool.category || 'write',
        timeout_ms: tool.timeout_ms ?? 30000,
        async: !!tool.async,
        enabled: tool.enabled
      })
      parametersRows.value = Array.isArray(tool.input_parameters) ? tool.input_parameters : []
      outputSchemaJson.value = JSON.stringify(tool.output_schema ?? {}, null, 2)
      const impl = tool.implementation || {}
      pythonCode.value = impl.code || ''
      // Normalize legacy values (e.g. "python311") to the locked set.
      const rawLang = String(impl.language || '').toLowerCase().trim()
      implLanguage.value = rawLang === 'javascript' ? 'javascript' : 'python'
      pythonPackages.value = impl.packages || ''
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    } finally {
      isLoading.value = false
    }
  }
})
</script>
