<script setup>
import { computed } from 'vue'
import { TagsInputItem, useForwardProps } from 'radix-vue'

import { cn } from '../../../lib/utils'

const props = defineProps({
  value: { type: [String, Object], required: true },
  disabled: { type: Boolean, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  color: { type: String, default: 'grey' },
  class: { type: null, required: false }
})

const delegatedProps = computed(() => {
  const { class: _, color: __, ...delegated } = props

  return delegated
})

const forwardedProps = useForwardProps(delegatedProps)

const colorClasses = computed(() => {
  const classes = {
    grey: 'bg-secondary text-secondary-foreground border',
    green: 'bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-100',
    red: 'bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-100',
    blue: 'bg-blue-100 text-blue-900 dark:bg-blue-950 dark:text-blue-100',
    yellow: 'bg-yellow-100 text-yellow-900 dark:bg-yellow-950 dark:text-yellow-100',
    purple: 'bg-purple-100 text-purple-900 dark:bg-purple-950 dark:text-purple-100',
    orange: 'bg-orange-100 text-orange-900 dark:bg-orange-950 dark:text-orange-100',
    teal: 'bg-teal-100 text-teal-900 dark:bg-teal-950 dark:text-teal-100'
  }
  return classes[props.color] || classes.grey
})
</script>

<template>
  <TagsInputItem
    v-bind="forwardedProps"
    :class="
      cn(
        'flex min-h-5 max-w-full items-center rounded-md data-[state=active]:ring-ring data-[state=active]:ring-2 data-[state=active]:ring-offset-2 ring-offset-background',
        colorClasses,
        props.class
      )
    "
  >
    <slot />
  </TagsInputItem>
</template>
