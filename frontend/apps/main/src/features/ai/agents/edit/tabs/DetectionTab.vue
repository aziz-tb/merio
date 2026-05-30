<template>
  <div class="px-6 py-6 max-w-4xl space-y-4">
    <div>
      <h2 class="text-lg font-semibold">{{ t('admin.ai.agent.tab.detection') }}</h2>
      <p class="text-sm text-muted-foreground mt-1">
        {{ t('admin.ai.agent.detectionHelpLong') }}
      </p>
    </div>

    <div v-if="!agent" class="text-sm text-muted-foreground">…</div>

    <DetectionBuilder
      v-else
      v-model="agent.detection"
      :available-sources="['ticket', 'customer', 'contract', 'tags']"
    />
  </div>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import DetectionBuilder from '@/features/admin/detection/DetectionBuilder.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

const { t } = useI18n()
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)
</script>
