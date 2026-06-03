<template>
  <PageHeader>
    <template #actions>
      <Button type="button" variant="outline" @click="cancel">
        {{ t('globals.messages.cancel') }}
      </Button>
      <Button type="submit" form="ai-tool-form" :is-loading="isLoading">
        {{ isNewForm ? t('globals.messages.create') : t('globals.messages.save') }}
      </Button>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading">
      <form id="ai-tool-form" class="space-y-8" @submit="onSubmit">
        <section class="space-y-4">
          <h2 class="text-xs font-semibold uppercase text-muted-foreground">
            {{ t('admin.ai.tool.section.basics') }}
          </h2>

          <FormField v-slot="{ field }" name="name">
            <FormItem>
              <FormLabel>{{ t('globals.terms.name') }}</FormLabel>
              <FormControl>
                <Input type="text" v-bind="field" @keydown.enter.prevent />
              </FormControl>
              <p v-if="isNewForm && autoSlugPreview" class="text-xs text-muted-foreground">
                {{ t('globals.terms.slug') }}:
                <span class="font-mono">{{ autoSlugPreview }}</span>
              </p>
              <FormMessage />
            </FormItem>
          </FormField>

          <FormItem v-if="!isNewForm">
            <Label class="text-muted-foreground">{{ t('globals.terms.slug') }}</Label>
            <Input
              type="text"
              :model-value="form.values.slug || ''"
              readonly
              tabindex="-1"
              class="cursor-not-allowed bg-muted/40"
            />
            <p class="text-sm text-muted-foreground">{{ t('admin.ai.slugPermanent') }}</p>
          </FormItem>

          <FormField v-slot="{ field }" name="description">
            <FormItem>
              <FormLabel>{{ t('globals.terms.description') }}</FormLabel>
              <FormControl>
                <Input type="text" v-bind="field" @keydown.enter.prevent />
              </FormControl>
              <FormMessage />
            </FormItem>
          </FormField>

          <div class="grid gap-5 md:grid-cols-3">
            <FormField v-slot="{ componentField }" name="category">
              <FormItem>
                <FormLabel>{{ t('globals.terms.category') }}</FormLabel>
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
                <FormMessage />
              </FormItem>
            </FormField>

            <FormField v-slot="{ field }" name="timeout_ms">
              <FormItem>
                <FormLabel>{{ t('globals.terms.timeoutMs') }}</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    step="1"
                    min="1"
                    max="120000"
                    v-bind="field"
                    @input="(e) => form.setFieldValue('timeout_ms', Number(e.target.value))"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            </FormField>

            <FormField v-slot="{ value, handleChange }" type="checkbox" name="async">
              <FormItem class="flex h-full flex-row items-center gap-x-3 space-y-0 pt-7">
                <FormControl>
                  <Checkbox :checked="value" @update:checked="handleChange" />
                </FormControl>
                <div class="space-y-1 leading-none">
                  <FormLabel class="text-foreground">{{ t('globals.terms.async') }}</FormLabel>
                  <FormMessage />
                </div>
              </FormItem>
            </FormField>
          </div>
        </section>

        <section class="space-y-4">
          <div>
            <h2 class="text-xs font-semibold uppercase text-muted-foreground">
              {{ t('admin.ai.tool.section.inputParameters') }}
            </h2>
            <p class="mt-1 text-sm text-muted-foreground">
              {{ t('admin.ai.tool.inputParametersHelp') }}
            </p>
          </div>
          <ToolParametersTable v-model="parametersRows" />
        </section>

        <section class="space-y-4">
          <h2 class="text-xs font-semibold uppercase text-muted-foreground">
            {{ t('admin.ai.tool.section.implementation') }}
          </h2>
          <CodeEditorPanel
            v-model:code="implCode"
            v-model:language="implLanguage"
            v-model:packages="implPackages"
            :title="t('globals.terms.implementationCode')"
            height-class="h-[36rem]"
          />
        </section>

        <section class="space-y-2">
          <FunctionPreviewPanel
            :name="functionNameForPreview"
            :code="implCode"
            :language="implLanguage"
            :packages="previewPackages"
            v-model:parameters="previewParams"
          />
        </section>
      </form>
  </LoadingOverlay>
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
  FormMessage
} from '@shared-ui/components/ui/form'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import CodeEditorPanel from '@/features/ai/functions/CodeEditorPanel.vue'
import FunctionPreviewPanel from '@/features/ai/functions/FunctionPreviewPanel.vue'
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
const parametersRows = ref([])
const previewParams = ref([])
const implCode = ref('')
const implLanguage = ref('python')
const implPackages = ref('')

const form = useForm({
  validationSchema: toTypedSchema(createToolFormSchema(t)),
  initialValues: {
    name: '',
    slug: '',
    description: '',
    category: 'write',
    timeout_ms: 30000,
    async: false
  }
})

const isNewForm = computed(() => !props.id)
const autoSlugPreview = computed(() => slugify(form.values.name || ''))
const functionNameForPreview = computed(() =>
  isNewForm.value ? autoSlugPreview.value : (form.values.slug || '')
)
const previewPackages = computed(() =>
  implLanguage.value === 'javascript' ? '' : implPackages.value
)

const onSubmit = form.handleSubmit(async (values) => {
  const payload = {
    ...values,
    enabled: true,
    input_parameters: (parametersRows.value || []).filter((r) => r && r.key),
    output_schema: {},
    implementation: {
      code: implCode.value,
      language: implLanguage.value,
      packages: previewPackages.value
    }
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

function cancel() {
  router.push({ name: 'ai-tool-list' })
}

onMounted(async () => {
  if (!props.id) return
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
      async: !!tool.async
    })
    parametersRows.value = Array.isArray(tool.input_parameters) ? tool.input_parameters : []
    const impl = tool.implementation || {}
    implCode.value = impl.code || ''
    const rawLang = String(impl.language || '').toLowerCase().trim()
    implLanguage.value = rawLang === 'javascript' ? 'javascript' : 'python'
    implPackages.value = impl.packages || ''
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
})
</script>
