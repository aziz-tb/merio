<template>
  <div class="space-y-2">
    <Label>{{ $t('admin.detection.mode') }}</Label>
    <Select v-model="mode">
      <SelectTrigger>
        <SelectValue :placeholder="t('admin.detection.selectMode')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="item in modes" :key="item.value" :value="item.value">
          {{ item.label }}
        </SelectItem>
      </SelectContent>
    </Select>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@shared-ui/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'

const model = defineModel({ required: true })
const { t } = useI18n()

const modes = computed(() => [
  { value: 'rules_only', label: t('admin.detection.mode.rulesOnly') },
  { value: 'ai_only', label: t('admin.detection.mode.aiOnly') },
  { value: 'rules_and_ai', label: t('admin.detection.mode.rulesAndAi') },
  { value: 'rules_or_ai', label: t('admin.detection.mode.rulesOrAi') },
  { value: 'manual_only', label: t('admin.detection.mode.manualOnly') }
])

const mode = computed({
  get: () => model.value,
  set: (value) => {
    model.value = value
  }
})
</script>
