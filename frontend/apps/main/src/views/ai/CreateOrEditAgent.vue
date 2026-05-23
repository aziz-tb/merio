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

      <!-- Slug is auto-derived from name on create and locked thereafter.
           Read-only display so authors see the value, but never editable;
           the backend rejects PUT requests that try to change it. -->
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

      <FormField v-slot="{ field }" name="prompt">
        <FormItem>
          <FormLabel>{{ $t('globals.terms.prompt') || 'Prompt' }}</FormLabel>
          <FormControl>
            <CodeEditor
              :model-value="field.value || ''"
              @update:model-value="form.setFieldValue('prompt', $event)"
              language="text"
              height-class="h-[28rem]"
            />
          </FormControl>
          <FormDescription>{{ $t('admin.ai.agent.promptHelp') }}</FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>

      <FormField v-slot="{ componentField }" name="min_legitimation">
        <FormItem>
          <FormLabel>{{ $t('admin.ai.agent.minLegitimation') }}</FormLabel>
          <FormControl>
            <Input
              type="number"
              step="1"
              min="0"
              v-bind="componentField"
              @input="(e) => form.setFieldValue('min_legitimation', Number(e.target.value))"
            />
          </FormControl>
          <FormDescription>{{ $t('admin.ai.agent.minLegitimationHelp') }}</FormDescription>
          <FormMessage />
        </FormItem>
      </FormField>

      <div class="space-y-2">
        <Label class="text-muted-foreground">
          {{ $t('globals.terms.presetOverrides') || 'Reasoning preset' }}
        </Label>
        <PresetSelect v-model="reasonPreset" />
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.agent.presetOverridesHelp') }}</p>
      </div>

      <FormField v-slot="{ value, handleChange }" type="checkbox" name="extended_thinking_enabled">
        <FormItem class="flex flex-row items-start gap-x-3 space-y-0">
          <FormControl>
            <Checkbox :checked="value" @update:checked="handleChange" />
          </FormControl>
          <div class="space-y-1 leading-none">
            <FormLabel class="text-foreground">{{ $t('admin.ai.agent.instructions.extendedThinking') }}</FormLabel>
            <FormDescription>{{ $t('admin.ai.agent.instructions.extendedThinkingHelp') }}</FormDescription>
            <p class="text-sm text-muted-foreground">{{ $t('admin.ai.agent.instructions.platformFinalizeHelp') }}</p>
            <FormMessage />
          </div>
        </FormItem>
      </FormField>

      <div class="space-y-2">
        <Label class="text-muted-foreground">{{ $t('globals.terms.tool', 2) }}</Label>
        <AgentToolsPicker v-model="agentTools" />
      </div>

      <div class="space-y-2">
        <Label class="text-muted-foreground">
          {{ $t('globals.terms.knowledgeBase') || 'Knowledge base' }}
        </Label>
        <KBScopePicker v-model="agentKbScope" />
      </div>

      <!-- Detection (routing rules) stays as a JSON editor — the
           condition shape mirrors automation_rules and ships with its
           own dedicated UI elsewhere; rebuilding it inline is out of
           scope for this batch. -->
      <div class="space-y-2">
        <Label class="text-muted-foreground">{{ $t('globals.terms.detection') }}</Label>
        <JsonField
          v-model="detectionJson"
          height-class="h-48"
          @parse-error="detectionParseError = $event"
        />
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.agent.detectionHelp') }}</p>
      </div>

        <Button type="submit" :isLoading="isLoading">
          {{ isNewForm ? $t('globals.messages.create') : $t('globals.messages.save') }}
        </Button>
      </form>

      <!-- Inline run panel — only on edit (agent must have an id).
           Hits POST /api/v1/ai/agents/:id/run; renders the runner's
           full RunResponse including the per-turn trace timeline. -->
      <AgentTestPanel
        v-if="!isNewForm && loadedAgent"
        :agent="loadedAgent"
      />
    </LoadingOverlay>
  </div>
</template>

<script setup>
import { onMounted, ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Button } from '@shared-ui/components/ui/button'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
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
import AgentTestPanel from '@/features/ai/agents/testing/AgentTestPanel.vue'
import AgentToolsPicker from '@/features/ai/agents/form/AgentToolsPicker.vue'
import KBScopePicker from '@/features/ai/agents/form/KbScopePicker.vue'
import PresetSelect from '@/features/ai/shared/PresetSelect.vue'
import { slugify } from '@/utils/slugify.js'
import { createAgentFormSchema } from '@/features/ai/agents/form/formSchema.js'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({ id: { type: [String, Number], required: false } })

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const emitter = useEmitter()

const isLoading = ref(false)
// Whole agent record kept in a ref so AgentTestPanel can read it
// (id, slug, enabled). Populated in onMounted when editing.
const loadedAgent = ref(null)

// Detection (routing rules) keeps its JSON editor — the rule shape
// matches automation_rules and the form for that lives elsewhere.
const detectionJson = ref('{}')
const detectionParseError = ref('')

// Picker state owned by the page; serialized into the agent payload at
// submit time.
const agentTools = ref([])
const agentKbScope = ref({ folder_ids: [], article_ids: [] })
// reasonPreset carries the slug or '' for "use platform default". The
// agent's preset_overrides JSONB is reduced to a single key ("reason")
// in v1 — the chunk D scope drops the full multi-purpose editor.
const reasonPreset = ref('')

const form = useForm({
  validationSchema: toTypedSchema(createAgentFormSchema(t)),
  initialValues: {
    name: '',
    slug: '',
    description: '',
    prompt: '',
    confidence_threshold: 0.5,
    extended_thinking_enabled: true,
    max_iterations: 5,
    min_legitimation: 0,
    enabled: true,
    kb_scope: { folder_ids: [], article_ids: [] }
  }
})

const isNewForm = computed(() => !props.id)

// Tiny client-side preview of the slug that the server will derive on
// create. Mirrors the canonical Go-side stringutil.Slugify.
const autoSlugPreview = computed(() => slugify(form.values.name || ''))

const breadcrumbPageLabel = () => (props.id ? t('aiAgent.edit') : t('aiAgent.new'))
const breadcrumbLinks = computed(() => [
  { path: 'ai-agent-list', label: t('globals.terms.aiAgent', 2) },
  { path: '', label: breadcrumbPageLabel() }
])

const onSubmit = form.handleSubmit(async (values) => {
  if (detectionParseError.value) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('admin.ai.invalidJson')
    })
    return
  }
  const presetOverrides = {}
  if (reasonPreset.value) presetOverrides.reason = reasonPreset.value
  let payload
  try {
    payload = {
      ...values,
      kb_scope: {
        folder_ids: (agentKbScope.value.folder_ids || []).map(Number),
        article_ids: (agentKbScope.value.article_ids || []).map(Number)
      },
      detection: detectionJson.value.trim() ? JSON.parse(detectionJson.value) : {},
      tools: agentTools.value,
      autonomy: {},
      preset_overrides: presetOverrides
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
      await api.updateAIAgent(props.id, payload)
    } else {
      // After create, drop the operator into the three-rail Edit
      // Agent layout's Identity tab. Falls back to the list page if
      // the response shape doesn't carry the new id for some reason.
      const created = await api.createAIAgent(payload)
      const newId = created?.data?.data?.id
      if (newId) {
        router.push({ name: 'edit-ai-agent-identity', params: { id: newId } })
      } else {
        router.push({ name: 'ai-agent-list' })
      }
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
  if (!props.id) return
  try {
    isLoading.value = true
    const resp = await api.getAIAgent(props.id)
    const a = resp.data.data
    loadedAgent.value = a
    form.setValues({
      name: a.name,
      slug: a.slug,
      description: a.description || '',
      prompt: a.prompt || '',
      confidence_threshold: Number(a.confidence_threshold),
      extended_thinking_enabled: a.extended_thinking_enabled !== false,
      max_iterations: a.max_iterations,
      min_legitimation: a.min_legitimation ?? 0,
      enabled: a.enabled,
      kb_scope: a.kb_scope || { folder_ids: [], article_ids: [] }
    })
    agentKbScope.value = a.kb_scope || { folder_ids: [], article_ids: [] }
    agentTools.value = Array.isArray(a.tools) ? a.tools : []
    reasonPreset.value = (a.preset_overrides && a.preset_overrides.reason) || ''
    detectionJson.value = JSON.stringify(a.detection ?? {}, null, 2)
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
