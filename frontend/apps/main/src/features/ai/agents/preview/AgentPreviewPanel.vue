<template>
  <aside class="w-full shrink-0 flex flex-col h-full overflow-hidden bg-background">
    <AgentPreviewHeader @close="$emit('close')" />

    <div class="px-4 pt-3 pb-2 border-b border-dashed border-gray-150 shrink-0">
      <PillSwitcher v-model="preview.channel.value">
        <PillSwitcherItem value="chat" :icon="MessageSquare">
          {{ t('globals.terms.chat') }}
        </PillSwitcherItem>
        <PillSwitcherItem value="email" :icon="Mail">
          {{ t('globals.terms.email', 1) }}
        </PillSwitcherItem>
      </PillSwitcher>
    </div>

    <div class="px-4 py-3 border-b border-input shrink-0 text-sm space-y-2.5">
      <div v-if="preview.channel.value === 'email'" class="flex items-center gap-3">
        <span class="text-xs text-muted-foreground w-14 shrink-0">{{ t('globals.terms.to') }}</span>
        <span class="text-muted-foreground">agent@libredesk</span>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-xs text-muted-foreground w-14 shrink-0">{{ t('globals.terms.from') }}</span>
        <AgentPreviewSourceSelect />
      </div>
      <div v-if="preview.channel.value === 'email'" class="flex items-center gap-3">
        <span class="text-xs text-muted-foreground w-14 shrink-0">{{ t('globals.terms.subject', 1) }}</span>
        <input
          v-model="preview.subject.value"
          type="text"
          class="flex-1 min-w-0 bg-transparent border-0 outline-none text-sm placeholder:text-muted-foreground"
          :placeholder="t('admin.ai.agent.preview.subjectPlaceholder')"
        />
      </div>
    </div>

    <div
      v-if="preview.linkedTicket.value"
      class="px-4 py-2 border-b border-input shrink-0 flex items-center justify-between gap-3 text-xs bg-muted/30"
    >
      <span class="inline-flex items-center gap-1.5 text-muted-foreground truncate min-w-0">
        <Link2 class="w-3 h-3 shrink-0" />
        <span class="truncate">
          {{ t('admin.ai.agent.preview.prefilledFrom') }}
          {{ linkedTicketLabel }}
        </span>
      </span>
      <Button variant="link" size="sm" class="h-auto p-0 shrink-0" @click="preview.reset()">
        {{ t('globals.messages.reset') }}
      </Button>
    </div>

    <div class="flex-1 overflow-auto px-4 py-3">
      <AgentRunResultPanel
        v-if="preview.lastRun.value || preview.streamStatus.value !== 'idle'"
        :result="preview.lastRun.value"
        :stream-events="preview.streamEvents.value"
        :stream-status="preview.streamStatus.value"
        :stream-error="preview.streamError.value"
        :is-streaming="['queued', 'starting', 'running', 'disconnected', 'failed'].includes(preview.streamStatus.value)"
      />
      <div
        v-else
        class="text-center text-sm text-muted-foreground py-12"
      >
        <Play class="w-5 h-5 mx-auto mb-2 text-muted-foreground/60" />
        <i18n-t keypath="admin.ai.agent.preview.emptyState" tag="p" scope="global">
          <template #channel>
            <strong class="text-foreground font-medium">{{ channelLabel }}</strong>
          </template>
        </i18n-t>
      </div>
    </div>

    <AgentPreviewComposer />
  </aside>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Link2, Mail, MessageSquare, Play } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { PillSwitcher, PillSwitcherItem } from '@shared-ui/components/ui/pill-switcher'
import AgentPreviewHeader from './AgentPreviewHeader.vue'
import AgentPreviewComposer from './AgentPreviewComposer.vue'
import AgentPreviewSourceSelect from './AgentPreviewSourceSelect.vue'
import AgentRunResultPanel from '@/features/ai/shared/RunResultPanel.vue'
import { AGENT_PREVIEW_KEY } from '@/composables/useAgentPreview.js'

defineEmits(['close'])

const { t } = useI18n()
const preview = inject(AGENT_PREVIEW_KEY)

const channelLabel = computed(() =>
  preview.channel.value === 'email'
    ? t('globals.terms.email', 1)
    : t('globals.terms.chat')
)

const linkedTicketLabel = computed(() => {
  const ticket = preview.linkedTicket.value
  if (!ticket) return ''
  const who = ticket.contact?.name || ticket.contact_name || ''
  return `#${ticket.reference_number || ticket.id}${who ? ' · ' + who : ''}`
})
</script>
