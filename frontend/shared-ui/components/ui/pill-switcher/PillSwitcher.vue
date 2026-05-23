<!--
  PillSwitcher — a two-state (or n-state) pill toggle for binary form
  state. NOT a tabbed interface: there are no content panels, just a
  segmented control whose value is part of a form.

  Use Tabs instead when you need TabsContent panels swapped by the
  selection. PillSwitcher is purely a v-model'd value selector.

  Usage:
    <PillSwitcher v-model="channel">
      <PillSwitcherItem value="chat" :icon="MessageSquare">Chat</PillSwitcherItem>
      <PillSwitcherItem value="email" :icon="Mail">Email</PillSwitcherItem>
    </PillSwitcher>
-->
<script setup>
import { computed, provide } from 'vue'
import { cn } from '../../../lib/utils'

const props = defineProps({
  modelValue: { type: [String, Number], required: false },
  class: { type: null, required: false },
})

const emits = defineEmits(['update:modelValue'])

provide('pillSwitcherContext', {
  current: computed(() => props.modelValue),
  select: (value) => emits('update:modelValue', value),
})
</script>

<template>
  <div
    :class="
      cn(
        'flex w-full items-center gap-1 rounded-lg bg-muted my-1 p-1',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
