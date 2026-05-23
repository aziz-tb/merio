<template>
  <div class="flex items-center justify-between gap-3 px-4 py-3 pb-4 border-b border-input shrink-0">
    <h3 class="text-lg font-medium text-foreground">
      {{ t('globals.terms.preview') }}
    </h3>
    <button
      type="button"
      class="text-muted-foreground hover:text-foreground transition-colors"
      :aria-label="t('globals.messages.close')"
      @click="$emit('close')"
    >
      <X class="w-4 h-4" />
    </button>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'

defineEmits(['close'])
const { t } = useI18n()
</script>
