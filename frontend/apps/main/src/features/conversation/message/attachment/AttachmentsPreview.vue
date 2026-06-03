<template>
  <div>
    <TransitionGroup name="attachment-list" tag="div" class="flex flex-wrap gap-2">
      <div
        v-for="attachment in allAttachments"
        :key="attachment.uuid || attachment.tempId"
        class="flex items-center gap-2 px-2.5 py-1.5 bg-background border border-border rounded-md hover:bg-muted transition-colors group max-w-xs"
      >
        <!-- Icon or loader -->
        <DotLoader v-if="attachment.loading" />
        <component
          v-else
          :is="getFileIcon(attachment.filename)"
          :size="16"
          class="text-muted-foreground flex-shrink-0"
        />

        <!-- Filename + size -->
        <Tooltip>
          <TooltipTrigger as-child>
            <div class="flex items-baseline gap-1.5 min-w-0">
              <span class="text-sm font-medium text-foreground truncate">
                {{ attachment.filename }}
              </span>
              <span class="text-xs text-muted-foreground flex-shrink-0">
                {{ formatBytes(attachment.size) }}
              </span>
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <p class="text-sm">{{ attachment.filename }}</p>
          </TooltipContent>
        </Tooltip>

        <!-- Delete button -->
        <button
          v-if="!attachment.loading"
          @click.prevent="onDelete(attachment.uuid)"
          class="ml-auto text-muted-foreground hover:text-destructive focus:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded transition-colors flex-shrink-0"
          :aria-label="`Remove ${attachment.filename}`"
        >
          <X :size="14" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { formatBytes } from '@shared-ui/utils/file'
import { getFileIcon } from '@/composables/useFileIcon'
import { X } from '@qall/icons'
import { DotLoader } from '@shared-ui/components/ui/loader'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'

const props = defineProps({
  attachments: {
    type: Array,
    required: true
  },
  uploadingFiles: {
    type: Array,
    default: () => []
  },
  onDelete: {
    type: Function,
    required: true
  }
})

const allAttachments = computed(() => [
  ...props.uploadingFiles.map((file, i) => ({
    tempId: `${file.name}-${i}`,
    filename: file.name,
    size: file.size,
    loading: true
  })),
  ...props.attachments
])
</script>

<style scoped>
.attachment-list-move,
.attachment-list-enter-active,
.attachment-list-leave-active {
  transition: all 0.3s ease;
}

.attachment-list-enter-from,
.attachment-list-leave-to {
  opacity: 0;
  transform: translateX(10px);
}

.attachment-list-leave-active {
  position: absolute;
}
</style>
