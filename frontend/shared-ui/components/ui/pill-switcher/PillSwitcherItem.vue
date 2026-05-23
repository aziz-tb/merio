<!--
  PillSwitcherItem — one segment of a PillSwitcher. Equal-width
  (flex-1), click selects its `value`. Renders an optional leading
  icon component plus the default slot as the label.
-->
<script setup>
import { computed, inject } from 'vue'
import { cn } from '../../../lib/utils'

const props = defineProps({
  value: { type: [String, Number], required: true },
  icon: { type: [Object, Function], required: false },
  disabled: { type: Boolean, required: false },
  class: { type: null, required: false },
})

const ctx = inject('pillSwitcherContext')
const isActive = computed(() => ctx?.current.value === props.value)
</script>

<template>
  <button
    type="button"
    :disabled="props.disabled"
    :class="
      cn(
        'flex flex-1 items-center justify-center gap-1.5 rounded-lg px-3 py-1 text-sm transition-all duration-150 ease-out disabled:pointer-events-none disabled:opacity-50',
        isActive
          ? 'bg-background font-medium text-foreground shadow-sm'
          : 'bg-transparent text-muted-foreground hover:text-foreground',
        props.class,
      )
    "
    @click="ctx?.select(props.value)"
  >
    <component :is="props.icon" v-if="props.icon" class="h-3.5 w-3.5" />
    <slot />
  </button>
</template>
