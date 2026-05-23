<template>
  <div class="space-y-2">
    <Label>{{ $t('admin.detection.aiPrompt') }}</Label>
    <Textarea
      v-model="prompt"
      rows="4"
      :placeholder="t('admin.detection.aiPromptPlaceholder')"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@shared-ui/components/ui/label'
import { Textarea } from '@shared-ui/components/ui/textarea'

const model = defineModel({ required: true })
const { t } = useI18n()

const prompt = computed({
  get: () => model.value,
  set: (value) => {
    model.value = value
  }
})
</script>
