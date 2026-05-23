<template>
  <div class="grid gap-2 mt-6 max-w-full [grid-template-columns:repeat(auto-fit,minmax(min(15rem,100%),15rem))]">
    <AttachmentTile
      v-for="attachment in attachments"
      :key="attachment.uuid"
      :attachment="attachment"
    />
  </div>
</template>

<script setup>
import AttachmentTile from '@/features/conversation/message/attachment/AttachmentTile.vue'

defineProps({
  attachments: {
    type: Array,
    required: true
  }
})
</script>
