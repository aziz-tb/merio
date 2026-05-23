<template>
  <Select :model-value="modelValueSafe" @update:modelValue="onChange">
    <SelectTrigger>
      <SelectValue :placeholder="$t('admin.ai.preset.placeholder')" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="__default__">
        {{ defaultLabel }}
      </SelectItem>
      <SelectItem v-for="p in presets" :key="p.slug" :value="p.slug">
        {{ p.name }} ({{ p.model }})
      </SelectItem>
    </SelectContent>
  </Select>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@shared-ui/components/ui/select'
import api from '@/api'

// PresetSelect renders the platform's reasoning-preset registry as a
// single-select dropdown. The "__default__" sentinel value means "no
// override" — the parent translates that to deleting the
// preset_overrides.reason key, so the runner falls back to the platform
// purpose mapping (config/ai_presets.json → purpose_mappings.reason).
//
// v-model carries the slug, or '' / null to mean "default."
const props = defineProps({
  modelValue: { type: [String, null], default: '' }
})
const emit = defineEmits(['update:modelValue'])

const { t } = useI18n()

const presets = ref([])
const defaultSlug = ref('default-sonnet')

const modelValueSafe = computed(() => {
  if (!props.modelValue) return '__default__'
  return props.modelValue
})

const defaultLabel = computed(
  () => `${t('admin.ai.preset.defaultLabelPrefix')} ${defaultSlug.value}`
)

const onChange = (v) => {
  if (v === '__default__') {
    emit('update:modelValue', '')
    return
  }
  emit('update:modelValue', v)
}

onMounted(async () => {
  try {
    const r = await api.getAIPresets()
    presets.value = r.data.data || []
    // Surface the canonical platform default slug in the label so the
    // operator knows what they get when "Default" is selected. Falls
    // back to the first enabled preset if the canonical name isn't
    // present (renamed in config/ai_presets.json).
    if (!presets.value.find((p) => p.slug === defaultSlug.value)) {
      defaultSlug.value = presets.value[0]?.slug || 'default'
    }
  } catch {
    presets.value = []
  }
})
</script>
