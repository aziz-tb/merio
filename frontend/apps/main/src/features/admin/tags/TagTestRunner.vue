<template>
  <div class="space-y-3 rounded-md border border-border p-4">
    <div>
      <h4 class="text-sm font-semibold">{{ t('tag.testRunner.title') }}</h4>
      <p class="text-xs text-muted-foreground">{{ t('tag.testRunner.description') }}</p>
    </div>

    <div class="flex gap-2">
      <Input
        v-model="conversationIdentifier"
        :placeholder="t('tag.testRunner.conversationUUID')"
      />
      <Button type="button" :disabled="!conversationIdentifier.trim() || running" @click="runTest">
        {{ running ? t('tag.testRunner.running') : t('tag.testRunner.run') }}
      </Button>
    </div>

    <div v-if="result" class="rounded-md border border-border p-3">
      <div :class="result.matched ? 'text-green-600' : 'text-red-600'" class="text-sm font-semibold">
        {{ result.matched ? t('tag.testRunner.success') : t('tag.testRunner.failure') }}
      </div>
      <div class="mt-1 text-xs text-muted-foreground">
        {{ t('tag.testRunner.score', { score: Number(result.score || 0).toFixed(2) }) }}
      </div>
      <div v-if="result.reasoning" class="mt-2 text-sm text-muted-foreground">
        {{ result.reasoning }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import api from '@/api'

const props = defineProps({
  tagId: { type: [Number, String], required: true }
})

const { t } = useI18n()
const emitter = useEmitter()
const conversationIdentifier = ref('')
const running = ref(false)
const result = ref(null)

const runTest = async () => {
  running.value = true
  result.value = null
  try {
    const response = await api.testTagDetection(props.tagId, {
      conversation_uuid: conversationIdentifier.value.trim()
    })
    result.value = response?.data?.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    running.value = false
  }
}
</script>
