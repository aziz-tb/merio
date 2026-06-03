<template>
  <div class="rounded-md border border-blue-200 bg-blue-50 p-2 text-xs text-blue-950 dark:border-blue-900/60 dark:bg-blue-950/40 dark:text-blue-100">
    <div class="flex items-start justify-between gap-2">
      <div class="flex min-w-0 gap-2">
        <Info class="mt-0.5 size-4 shrink-0" />
        <p class="whitespace-pre-wrap break-words">
          {{ t('conversation.contactData.conflict.title', { previousId, currentId }) }}
        </p>
      </div>
      <button
        type="button"
        class="rounded p-0.5 text-blue-800 hover:bg-blue-100 dark:text-blue-100 dark:hover:bg-blue-900/40"
        @click="$emit('dismiss')"
      >
        <X class="size-4" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { Info, X } from '@qall/icons'
import { useI18n } from 'vue-i18n'

defineProps({
  previousId: { type: String, required: true },
  currentId: { type: String, required: true },
})

defineEmits(['dismiss'])

const { t } = useI18n()
</script>
