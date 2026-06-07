<template>
  <Select :model-value="modelValueSafe" @update:modelValue="onChange">
    <SelectTrigger>
      <SelectValue :placeholder="$t('admin.ai.preset.placeholder')" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="__default__" :text-value="defaultLabel">
        {{ defaultLabel }}
      </SelectItem>
      <SelectItem
        v-for="p in visiblePresets"
        :key="p.slug"
        :value="p.slug"
        :text-value="p.name"
      >
        <span class="flex min-w-0 items-center gap-2">
          <img
            v-if="providerLogo(p.provider)"
            :src="providerLogo(p.provider)"
            alt=""
            aria-hidden="true"
            class="h-4 w-4 shrink-0"
          >
          <span class="truncate">{{ p.name }}</span>
        </span>
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

// PresetSelect renders the platform's preset registry as a single-select
// dropdown, scoped to one use case via the `purpose` prop. The
// "__default__" sentinel value means "no override" — the parent
// translates that to deleting the matching preset_overrides[purpose] key,
// so the runner falls back to the platform purpose mapping
// (config/ai_presets.json → purpose_mappings[purpose]).
//
// v-model carries the slug, or '' / null to mean "default."
//
// `purpose` selects which presets are offered: only presets whose config
// `visibility` array includes this value appear. This lets the same
// component back different use-case pickers (reason, rewrite, …) by
// passing a different purpose.
const props = defineProps({
  modelValue: { type: [String, null], default: '' },
  purpose: { type: String, default: 'reason' }
})
const emit = defineEmits(['update:modelValue'])

const { t } = useI18n()

const presets = ref([])
const providerLogos = {
  anthropic: '/images/claude-logo.svg',
  openai: '/images/openai-logo.svg',
  azure: '/images/azure-logo.svg'
}

const modelValueSafe = computed(() => {
  if (!props.modelValue) return '__default__'
  return props.modelValue
})

const defaultLabel = computed(() => t('admin.ai.preset.defaultLabelPrefix'))

const visiblePresets = computed(() =>
  presets.value.filter(
    (p) => p.enabled !== false && Array.isArray(p.visibility) && p.visibility.includes(props.purpose)
  )
)

const onChange = (v) => {
  if (v === '__default__') {
    emit('update:modelValue', '')
    return
  }
  emit('update:modelValue', v)
}

const providerLogo = (provider) => providerLogos[provider] || ''

onMounted(async () => {
  try {
    const r = await api.getAIPresets()
    presets.value = r.data.data || []
  } catch {
    presets.value = []
  }
})
</script>
