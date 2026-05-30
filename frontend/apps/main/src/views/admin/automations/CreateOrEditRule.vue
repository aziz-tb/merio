<template>
  <div class="mb-5">
    <CustomBreadcrumb :links="breadcrumbLinks" />
  </div>
  <LoadingOverlay :loading="isLoading">
    <form @submit="onSubmit">
      <div class="space-y-5">
        <FormField v-if="!isNewForm" v-slot="{ value, handleChange }" type="checkbox" name="enabled">
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

        <FormField v-slot="{ field }" name="description">
          <FormItem>
            <FormLabel>{{ $t('globals.terms.description') }}</FormLabel>
            <FormControl>
              <Input type="text" v-bind="field" @keydown.enter.prevent />
            </FormControl>
            <FormMessage />
          </FormItem>
        </FormField>

        <FormField v-slot="{ componentField, handleInput }" name="type">
          <FormItem>
            <FormLabel>{{ $t('globals.terms.type') }}</FormLabel>
            <FormControl>
              <Select v-bind="componentField" @update:modelValue="handleInput">
                <SelectTrigger>
                  <SelectValue :placeholder="t('placeholders.selectType')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="new_conversation">{{ $t('conversation.newConversation') }}</SelectItem>
                    <SelectItem value="conversation_update">{{ $t('admin.automation.conversationUpdate') }}</SelectItem>
                    <SelectItem value="time_trigger">{{ $t('admin.automation.timeTriggers') }}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </FormControl>
            <FormMessage />
          </FormItem>
        </FormField>

        <div :class="{ hidden: form.values.type !== 'conversation_update' }">
          <FormField v-slot="{ componentField, handleChange }" name="events">
            <FormItem>
              <FormLabel>{{ $t('globals.terms.event', 2) }}</FormLabel>
              <FormControl>
                <SelectTag
                  v-model="componentField.modelValue"
                  :items="conversationEventOptions"
                  :placeholder="t('placeholders.selectEvents')"
                  @update:modelValue="handleChange"
                />
              </FormControl>
              <FormDescription>{{ $t('admin.automation.evaluateRuleOnTheseEvents') }}</FormDescription>
              <FormMessage />
            </FormItem>
          </FormField>
        </div>

        <p class="font-semibold">{{ $t('admin.automation.matchTheseRules') }}</p>
        <DetectionBuilder
          v-if="form.values.type"
          v-model="rule.detection"
          :available-sources="['ticket', 'customer', 'contract', 'tags']"
        />

        <p class="font-semibold pt-2">{{ $t('admin.automation.performTheseActions') }}</p>
        <ActionBox
          :actions="rule.actions"
          :update-actions="handleUpdateActions"
          @add-action="handleAddAction"
          @remove-action="handleRemoveAction"
        />

        <Button type="submit" :isLoading="isLoading">
          {{ isNewForm ? $t('globals.messages.create') : $t('globals.messages.save') }}
        </Button>
      </div>
    </form>
  </LoadingOverlay>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button'
import { CustomBreadcrumb } from '@shared-ui/components/ui/breadcrumb'
import { SelectTag } from '@shared-ui/components/ui/select'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@shared-ui/components/ui/form'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import ActionBox from '@/features/admin/automation/ActionBox.vue'
import DetectionBuilder from '@/features/admin/detection/DetectionBuilder.vue'
import { createFormSchema } from '@/features/admin/automation/formSchema.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { useEmitter } from '@/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  id: {
    type: [String, Number],
    required: false
  }
})

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const emitter = useEmitter()
const isLoading = ref(false)

const defaultDetection = () => ({
  mode: 'rules_only',
  rules: {
    op: 'AND',
    children: []
  },
  ai: {
    prompt: ''
  }
})

const rule = ref({
  id: 0,
  name: '',
  description: '',
  type: 'new_conversation',
  events: [],
  enabled: true,
  execution_mode: 'all',
  detection: defaultDetection(),
  actions: []
})

const conversationEventOptions = [
  { label: t('conversation.agentAssigned'), value: 'conversation.user.assigned' },
  { label: t('conversation.teamAssigned'), value: 'conversation.team.assigned' },
  { label: t('admin.automation.event.priority.change'), value: 'conversation.priority.change' },
  { label: t('admin.automation.event.status.change'), value: 'conversation.status.change' },
  { label: t('admin.automation.event.message.outgoing'), value: 'conversation.message.outgoing' },
  { label: t('admin.automation.event.message.incoming'), value: 'conversation.message.incoming' }
]

const isNewForm = computed(() => !props.id)
const breadcrumbPageLabel = () => props.id > 0 ? t('automation.editRule') : t('automation.newRule')
const breadcrumbLinks = [
  { path: 'automation-list', label: t('globals.terms.automation') },
  { path: '', label: breadcrumbPageLabel() }
]

const form = useForm({
  validationSchema: toTypedSchema(createFormSchema(t)),
  initialValues: {
    name: '',
    description: '',
    enabled: true,
    type: 'new_conversation',
    events: []
  }
})

const onSubmit = form.handleSubmit(async (values) => {
  await handleSave(values)
})

const handleSave = async (values) => {
  const validationError = getValidationError()
  if (validationError) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'warning',
      description: validationError
    })
    return
  }

  try {
    isLoading.value = true
    const payload = buildPayload(values)
    if (props.id > 0) {
      await api.updateAutomationRule(props.id, payload)
    } else {
      await api.createAutomationRule(payload)
      router.push({ name: 'automation-list' })
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
}

const buildPayload = (values) => {
  const events = values.type === 'conversation_update' ? values.events || [] : []
  const ruleJSON = [{
    type: values.type,
    execution_mode: rule.value.execution_mode || 'all',
    event: events,
    detection: rule.value.detection,
    actions: rule.value.actions
  }]

  return {
    ...rule.value,
    ...values,
    events,
    rules: ruleJSON
  }
}

const getValidationError = () => {
  if (rulesRequired(rule.value.detection) && !hasCondition(rule.value.detection.rules)) {
    return t('admin.automation.validation.addCondition')
  }
  const conditionError = validateTree(rule.value.detection.rules)
  if (conditionError) return conditionError

  if (aiRequired(rule.value.detection) && !rule.value.detection.ai?.prompt) {
    return t('admin.detection.validation.aiPrompt')
  }

  if (rule.value.actions.length === 0) {
    return t('admin.automation.validation.addAction')
  }
  for (const action of rule.value.actions) {
    if (!action.type) return t('admin.automation.validation.selectActionType')
    if (action.type === 'send_csat') action.value = ['0']
    if (!Array.isArray(action.value) || action.value.length === 0) {
      return t('admin.automation.validation.setActionValue')
    }
    if (action.value.some((value) => !value)) {
      return t('admin.automation.validation.setActionValue')
    }
  }
  return ''
}

const rulesRequired = (detection) => !['ai_only', 'manual_only'].includes(detection.mode)
const aiRequired = (detection) => ['ai_only', 'rules_and_ai', 'rules_or_ai'].includes(detection.mode)

const hasCondition = (tree) => {
  if (!tree?.children?.length) return false
  return tree.children.some((child) => child.leaf || hasCondition(child.tree))
}

const validateTree = (tree) => {
  if (!tree?.children) return ''
  for (const child of tree.children) {
    if (child.tree) {
      const err = validateTree(child.tree)
      if (err) return err
    }
    if (child.leaf) {
      const leaf = child.leaf
      if (!leaf.source) return t('admin.detection.validation.selectSource')
      if (leaf.source !== 'tags' && !leaf.path) return t('admin.detection.validation.selectPath')
      if (!leaf.operator) return t('admin.automation.validation.selectOperator')
      if (!['is null', 'is not null'].includes(leaf.operator)) {
        if (Array.isArray(leaf.value) && leaf.value.length === 0) {
          return t('admin.automation.validation.setConditionValue')
        }
        if (!Array.isArray(leaf.value) && !leaf.value) {
          return t('admin.automation.validation.setConditionValue')
        }
      }
    }
  }
  return ''
}

const handleUpdateActions = (value, index) => {
  rule.value.actions[index] = value.value[index]
}

const handleAddAction = () => {
  rule.value.actions.push({ type: '', value: [] })
}

const handleRemoveAction = (index) => {
  rule.value.actions.splice(index, 1)
}

const normalizeRuleRecord = (record) => {
  const firstRule = Array.isArray(record.rules) && record.rules.length > 0 ? record.rules[0] : null
  return {
    ...record,
    events: record.events || firstRule?.event || [],
    execution_mode: record.execution_mode || firstRule?.execution_mode || 'all',
    detection: firstRule?.detection || defaultDetection(),
    actions: firstRule?.actions || []
  }
}

onMounted(async () => {
  if (props.id > 0) {
    try {
      isLoading.value = true
      const resp = await api.getAutomationRule(props.id)
      rule.value = normalizeRuleRecord(resp.data.data)
      form.setValues(rule.value)
    } catch (error) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(error).message
      })
    } finally {
      isLoading.value = false
    }
  } else {
    if (route.query.type) {
      rule.value.type = route.query.type
      form.setFieldValue('type', route.query.type)
    } else {
      form.setValues(rule.value)
    }
  }
})
</script>
