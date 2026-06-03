<template>
  <Button
    variant="ghost"
    @click.stop="onClose"
    size="xs"
    class="text-muted-foreground hover:text-foreground hover:bg-accent w-6 h-6 p-0"
    :aria-label="ariaLabel"
  >
    <slot>
      <X size="16" />
    </slot>
  </Button>
</template>

<script setup>
import { Button } from '@shared-ui/components/ui/button'
import { X } from '@qall/icons'

defineProps({
  onClose: {
    type: Function,
    required: true
  },
  ariaLabel: {
    type: String,
    default: 'Close'
  }
})
</script>
