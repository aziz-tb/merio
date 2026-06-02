<script setup>
import { computed } from 'vue'
import { TagsInputItem, useForwardProps } from 'radix-vue'

import { getTagPillClass } from '../../../utils/tag'

const props = defineProps({
  value: { type: [String, Object], required: true },
  disabled: { type: Boolean, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  color: { type: String, default: 'grey' },
  class: { type: null, required: false }
})

const delegatedProps = computed(() => {
  const delegated = { ...props }
  delete delegated.class
  delete delegated.color

  return delegated
})

const forwardedProps = useForwardProps(delegatedProps)

const itemClass = computed(() =>
  getTagPillClass(props.color, {
    size: 'input',
    class: [
      'data-[state=active]:ring-ring data-[state=active]:ring-2 data-[state=active]:ring-offset-2 ring-offset-background',
      props.class
    ]
  })
)
</script>

<template>
  <TagsInputItem
    v-bind="forwardedProps"
    :class="itemClass"
  >
    <slot />
  </TagsInputItem>
</template>
