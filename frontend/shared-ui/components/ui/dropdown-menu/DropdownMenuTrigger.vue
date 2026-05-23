<script setup>
import { computed } from 'vue'
import { DropdownMenuTrigger, useForwardProps } from 'radix-vue'
import { cn } from '@shared-ui/lib/utils'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  disabled: { type: Boolean, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  class: { type: String, required: false }
})

const delegatedProps = computed(() => {
  const { class: _, ...rest } = props
  return rest
})

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <DropdownMenuTrigger
    :class="cn('outline-none', props.class)"
    v-bind="forwardedProps"
  >
    <slot />
  </DropdownMenuTrigger>
</template>
