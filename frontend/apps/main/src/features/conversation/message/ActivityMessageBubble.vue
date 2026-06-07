<!-- ActivityMessageBubble.vue -->
<template>
  <div class="text-center">
    <div class="text-muted-foreground text-sm my-2">
      {{ message.content }}
      <Tooltip>
        <TooltipTrigger>
          <span class="text-xs ml-1">{{ formatDateTime(message.updated_at) }}</span>
        </TooltipTrigger>
        <TooltipContent>
          <p>
            {{ formatDateTime(message.updated_at) }}
          </p>
        </TooltipContent>
      </Tooltip>
    </div>
  </div>
</template>

<script setup>
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { formatDateTime } from '@main/utils/date'

defineProps({
  message: Object
})
</script>
