<template>
  <Button
    type="button"
    variant="ghost"
    size="icon"
    :class="cn('cursor-pointer h-7 w-7', props.class)"
    @click="toggle"
  >
    <slot>
      <LayoutLeft class="size-[1.1rem]" />
    </slot>
    <span class="sr-only">{{ label }}</span>
  </Button>
</template>

<script setup>
import { Button } from '@shared-ui/components/ui/button'
import { cn } from '@shared-ui/lib/utils'
import { LayoutLeft } from '@qall/icons'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'

const props = defineProps({
  class: { type: null, required: false },
  label: { type: String, default: 'Toggle sidebar' }
})

const { toggle } = useLayoutSidebar()
</script>
