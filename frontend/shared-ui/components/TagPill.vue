<script setup>
import { computed } from 'vue'
import { getTagPillClass } from '../utils/tag'

const props = defineProps({
  tag: { type: Object, default: null },
  label: { type: String, default: '' },
  color: { type: String, default: '' },
  size: { type: String, default: 'sm' },
  as: { type: String, default: 'span' },
  title: { type: String, default: '' },
  class: { type: null, required: false }
})

const displayLabel = computed(() => props.label || props.tag?.name || '')
const displayColor = computed(() => props.color || props.tag?.color || 'grey')
const titleText = computed(() => props.title || displayLabel.value)
const pillClass = computed(() =>
  getTagPillClass(displayColor.value, {
    size: props.size,
    class: props.class
  })
)
</script>

<template>
  <component :is="as" :class="pillClass" :title="titleText || undefined">
    <slot :label="displayLabel">
      <span class="truncate">{{ displayLabel }}</span>
    </slot>
  </component>
</template>
