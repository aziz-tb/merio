<template>
  <div class="inline-flex items-center gap-0.5 rounded-full border border-border bg-white dark:bg-accent border-secondary p-1 shadow-md">
    <Tooltip>
      <TooltipTrigger as-child>
        <button
          type="button"
          :class="buttonClass(INBOX_VIEW_MODES.COLUMNS)"
          :aria-pressed="viewMode === INBOX_VIEW_MODES.COLUMNS"
          @click="switchToColumns"
        >
          <Columns3 class="size-5" />
          <span class="sr-only">{{ t('conversation.table.viewColumns') }}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top">
        <p>{{ t('conversation.table.viewColumns') }}</p>
      </TooltipContent>
    </Tooltip>

    <Tooltip>
      <TooltipTrigger as-child>
        <button
          type="button"
          :class="buttonClass(INBOX_VIEW_MODES.TABLE)"
          :aria-pressed="viewMode === INBOX_VIEW_MODES.TABLE"
          @click="switchToTable"
        >
          <PanelLeft class="size-5" />
          <span class="sr-only">{{ t('conversation.table.viewTable') }}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top">
        <p>{{ t('conversation.table.viewTable') }}</p>
      </TooltipContent>
    </Tooltip>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { PanelLeft, Columns3 } from 'lucide-vue-next'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { INBOX_VIEW_MODES, useInboxView } from '@main/composables/useInboxView'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { viewMode, setViewMode } = useInboxView()

const conversationRoutes = ['inbox-conversation', 'team-inbox-conversation', 'view-inbox-conversation']

const listRouteForCurrent = () => {
  if (route.name === 'team-inbox-conversation') {
    return { name: 'team-inbox', params: { teamID: route.params.teamID } }
  }
  if (route.name === 'view-inbox-conversation') {
    return { name: 'view-inbox', params: { viewID: route.params.viewID } }
  }
  return { name: 'inbox', params: { type: route.params.type } }
}

const switchToTable = () => {
  // When entering table view from an open conversation, drop the conversation
  // out of the URL so the drawer doesn't auto-open.
  if (conversationRoutes.includes(route.name)) {
    router.push(listRouteForCurrent())
  }
  setViewMode(INBOX_VIEW_MODES.TABLE)
}

const switchToColumns = () => {
  setViewMode(INBOX_VIEW_MODES.COLUMNS)
}

const buttonClass = (mode) => [
  'inline-flex h-7 w-8 p-1 items-center justify-center rounded-full border text-muted-foreground transition-colors',
  viewMode.value === mode
    ? 'border-border bg-muted/90 text-foreground shadow-sm'
    : 'border-transparent hover:text-foreground'
]
</script>