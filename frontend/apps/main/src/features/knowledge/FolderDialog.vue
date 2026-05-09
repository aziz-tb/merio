<template>
  <Dialog :open="open" @update:open="(v) => emit('update:open', v)">
    <DialogContent class="sm:max-w-[425px]">
      <DialogHeader>
        <DialogTitle>
          {{ mode === 'create' ? t('admin.knowledge.actions.newFolder') : t('admin.knowledge.actions.rename') }}
        </DialogTitle>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="onSubmit">
        <FormField v-slot="{ componentField }" name="name">
          <FormItem>
            <FormLabel>{{ t('globals.terms.name') }}</FormLabel>
            <FormControl>
              <Input v-bind="componentField" />
            </FormControl>
            <FormMessage />
          </FormItem>
        </FormField>
        <DialogFooter>
          <Button type="button" variant="ghost" @click="emit('update:open', false)">
            {{ t('globals.messages.cancel') }}
          </Button>
          <Button type="submit">
            {{ mode === 'create' ? t('globals.messages.create') : t('globals.messages.save') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog/index.js'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@shared-ui/components/ui/form'
import { Input } from '@shared-ui/components/ui/input'
import { Button } from '@shared-ui/components/ui/button/index.js'
import { createFolderSchema } from './formSchema.js'
import { slugify } from './utils/slugify.js'

const { t } = useI18n()

const props = defineProps({
  open: { type: Boolean, default: false },
  mode: { type: String, default: 'create' }, // 'create' | 'rename'
  initial: { type: Object, default: () => ({ name: '' }) }
})
const emit = defineEmits(['update:open', 'submit'])

const form = useForm({
  validationSchema: toTypedSchema(createFolderSchema(t)),
  initialValues: { name: props.initial.name || '' }
})

watch(
  () => [props.open, props.initial],
  ([isOpen]) => {
    if (isOpen) {
      form.resetForm({ values: { name: props.initial.name || '' } })
    }
  },
  { immediate: true, deep: true }
)

// Slug is auto-derived from the name on create. On rename the slug never
// changes (server keeps the original); we send empty and the server ignores.
const onSubmit = form.handleSubmit((values) => {
  const slug = props.mode === 'create' ? slugify(values.name) : ''
  emit('submit', { name: values.name, slug })
})
</script>
