<script setup>
import { computed } from 'vue'
import {
  getStatusName,
  getStatusPillClass,
  getStatusVisual,
  STATUS_ICONS,
  STATUS_ICON_SIZE_CLASSES
} from '@main/utils/statusVisuals'

const props = defineProps({
  status: { type: Object, default: null },
  label: { type: String, default: '' },
  color: { type: String, default: '' },
  icon: { type: String, default: '' },
  size: { type: String, default: 'md' },
  as: { type: String, default: 'span' },
  title: { type: String, default: '' },
  showIcon: { type: Boolean, default: true },
  class: { type: null, required: false }
})

const statusInput = computed(() => ({
  ...props.status,
  name: props.status?.name || props.status?.label || props.label,
  label: props.status?.label || props.status?.name || props.label,
  color: props.color || props.status?.color,
  icon: props.icon || props.status?.icon
}))
const displayLabel = computed(() => props.label || getStatusName(statusInput.value))
const visual = computed(() => getStatusVisual(statusInput.value))
const iconComponent = computed(() => STATUS_ICONS[visual.value.icon])
const titleText = computed(() => props.title || displayLabel.value)
const pillClass = computed(() =>
  getStatusPillClass(visual.value.color, {
    size: props.size,
    class: props.class
  })
)
const iconClass = computed(() => STATUS_ICON_SIZE_CLASSES[props.size] || STATUS_ICON_SIZE_CLASSES.sm)
</script>

<template>
  <component :is="as" :class="pillClass" :title="titleText || undefined">
    <component
      v-if="showIcon && iconComponent"
      :is="iconComponent"
      :class="[iconClass, 'shrink-0']"
    />
    <slot :label="displayLabel">
      <span class="truncate">{{ displayLabel }}</span>
    </slot>
  </component>
</template>
