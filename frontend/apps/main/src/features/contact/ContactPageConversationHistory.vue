<template>
  <Card class="p-4">
    <h3 class="mb-3 text-sm font-semibold">{{ t('contact.page.conversationHistory') }}</h3>

    <div v-if="loading && items.length === 0" class="space-y-3">
      <div v-for="i in 3" :key="i" class="animate-pulse">
        <div class="mb-1 h-4 w-3/4 rounded bg-muted" />
        <div class="h-3 w-1/2 rounded bg-muted/70" />
      </div>
    </div>

    <div v-else-if="items.length === 0" class="py-6 text-center">
      <p class="text-sm text-muted-foreground">{{ t('contact.page.noPreviousConversations') }}</p>
    </div>

    <div v-else class="space-y-1">
      <router-link
        v-for="conversation in items"
        :key="conversation.uuid"
        :to="conversationRoute(conversation)"
        class="block rounded-md p-2 transition-colors hover:bg-muted/60"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium">
              {{ stringValue(conversation.subject) || t('contact.page.noSubject') }}
            </p>
            <p v-if="stringValue(conversation.last_contact_message)" class="mt-0.5 truncate text-xs text-muted-foreground">
              {{ stringValue(conversation.last_contact_message) }}
            </p>
          </div>
          <div class="mt-0.5 flex shrink-0 items-center gap-2 text-right">
            <Avatar v-if="assigneeName(conversation)" class="size-5">
              <AvatarImage
                v-if="stringValue(conversation.assignee?.avatar_url)"
                :src="stringValue(conversation.assignee.avatar_url)"
                :alt="assigneeName(conversation)"
              />
              <AvatarFallback class="text-[10px]">
                {{ initials(assigneeName(conversation)) }}
              </AvatarFallback>
            </Avatar>

            <component
              :is="priorityIcon(conversation)"
              v-if="priorityIcon(conversation)"
              class="size-3 shrink-0"
            />

            <span
              v-if="stringValue(conversation.status)"
              class="whitespace-nowrap text-xs text-muted-foreground"
              :style="{ color: stringValue(conversation.status_color) || undefined }"
            >
              ● {{ stringValue(conversation.status) }}
            </span>

            <span class="whitespace-nowrap text-xs text-muted-foreground">
              {{ relativeTime(conversation.last_message_at || conversation.created_at) }}
            </span>
          </div>
        </div>
      </router-link>

      <Button
        v-if="hasMore"
        type="button"
        variant="ghost"
        size="sm"
        class="mt-3 w-full text-xs"
        :disabled="loading"
        @click="$emit('loadMore')"
      >
        {{ loading ? t('globals.messages.loading') : t('globals.terms.loadMore') }}
      </Button>
    </div>
  </Card>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import { Button } from '@shared-ui/components/ui/button'
import { Card } from '@shared-ui/components/ui/card'
import { getRelativeTime } from '@shared-ui/utils/datetime.js'
import { getInitials } from '@shared-ui/utils/string'
import { PRIORITY_ICONS, getPriorityVisual } from '@/utils/priorityVisuals'

const props = defineProps({
  items: { type: Array, default: () => [] },
  total: { type: Number, default: 0 },
  page: { type: Number, default: 1 },
  pageSize: { type: Number, default: 20 },
  loading: { type: Boolean, default: false }
})

defineEmits(['loadMore'])

const { t } = useI18n()

const hasMore = computed(() => props.items.length < props.total)

function conversationRoute (conversation) {
  return {
    name: 'inbox-conversation',
    params: {
      type: 'assigned',
      uuid: conversation.uuid
    }
  }
}

function stringValue (value) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (typeof value === 'object') {
    if (value.Valid === false || value.valid === false) return ''
    return value.String || value.string || value.Value || value.value || ''
  }
  return ''
}

function relativeTime (value) {
  const raw = stringValue(value) || value
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return getRelativeTime(date)
}

function assigneeName (conversation) {
  return stringValue(conversation.assignee?.name).trim()
}

function initials (name) {
  const parts = String(name || '').trim().split(/\s+/)
  return getInitials(parts[0] || '', parts.slice(1).join(' '))
}

function priorityIcon (conversation) {
  const priorityName = stringValue(conversation.priority_name)
  if (!priorityName) return null
  const visual = getPriorityVisual(priorityName)
  return PRIORITY_ICONS[visual.icon] || null
}
</script>
