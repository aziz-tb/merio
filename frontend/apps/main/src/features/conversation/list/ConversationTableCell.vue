<!--  frontend/apps/main/src/features/conversation/list/ConversationTableCell.vue -->

<template>
  <!-- Subject column: subject (bold) + last message preview (muted) stacked -->
  <div
    v-if="columnId === 'subject'"
    class="flex min-w-0 flex-col gap-0.5 m-3"
    :title="subjectText"
  >
    <div class="truncate text-foreground text-sm font-medium">
      {{ subjectText || missing }}
    </div>
    <p class="truncate text-muted-foreground text-xs" :title="activityText">
      {{ activityText || missing }}
    </p>
  </div>

  <!-- User column: avatar + contact name + email stacked -->
  <div
    v-else-if="columnId === 'user'"
    class="flex min-w-0 items-center gap-2.5"
    :title="contactTitle"
  >
    <Avatar class="size-7 shrink-0 rounded-full">
      <AvatarImage :src="contactAvatar" class="object-cover" />
      <AvatarFallback class="text-[10px] font-medium">
        {{ contactInitials }}
      </AvatarFallback>
    </Avatar>
    <div class="min-w-0 flex flex-col gap-0.5">
      <div class="flex min-w-0 items-center gap-1.5">
        <span class="truncate text-foreground text-sm">{{ displayName || missing }}</span>
        <Tooltip v-if="isLinked">
          <TooltipTrigger as-child>
            <Link class="size-3.5 shrink-0 text-muted-foreground" />
          </TooltipTrigger>
          <TooltipContent>{{ t('conversation.customer.linkedTooltip') }}</TooltipContent>
        </Tooltip>
      </div>
      <p
        v-if="contactEmail"
        class="truncate text-muted-foreground text-xs"
        :title="contactEmail"
      >
        {{ contactEmail }}
      </p>
    </div>
  </div>

  <div
    v-else-if="columnId === 'priority'"
    :class="[editableCellClass, 'justify-center']"
    @click.stop
  >
    <SelectComboBox
      :model-value="priorityModelValue"
      :items="priorityOptions"
      :placeholder="t('globals.terms.none')"
      @select="selectPriority"
      type="priority"
      trigger-variant="inline"
      button-class="w-full justify-center px-2 py-1"
    />
  </div>

  <span
    v-else-if="columnId === 'waiting_since'"
    class="text-muted-foreground tabular-nums flex justify-center"
    :title="absoluteTitle(conversation.waiting_since)"
  >
    {{ compactRelative(conversation.waiting_since) }}
  </span>

  <span
    v-else-if="columnId === 'last_activity'"
    class="text-muted-foreground tabular-nums flex justify-center"
    :title="absoluteTitle(lastActivityAt)"
  >
    {{ compactRelative(lastActivityAt) }}
  </span>

  <span
    v-else-if="columnId === 'next_sla'"
    class="text-muted-foreground tabular-nums flex justify-center"
    :title="absoluteTitle(conversation.next_sla_deadline_at)"
  >
    {{ compactRelative(conversation.next_sla_deadline_at) }}
  </span>

  <div
    v-else-if="columnId === 'status'"
    :class="editableCellClass"
    @click.stop
  >
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <StatusPill
          as="button"
          type="button"
          :status="currentStatusOption"
          :label="statusDisplayLabel(currentStatusOption)"
          size="sm"
          class="cursor-pointer justify-center transition-opacity hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-1"
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="center" class="min-w-44" @click.stop>
        <DropdownMenuItem
          v-for="status in statusOptions"
          :key="status.value"
          class="gap-2"
          @click.stop="selectStatus(status)"
        >
          <StatusPill :status="status" :label="statusDisplayLabel(status)" size="sm" />
          <Check
            :class="[
              'ml-auto size-4',
              isSelectedStatus(status) ? 'opacity-100' : 'opacity-0'
            ]"
          />
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>

  <div
    v-else-if="columnId === 'team'"
    :class="editableCellClass"
    @click.stop
  >
    <SelectComboBox
      :model-value="teamModelValue"
      :items="teamOptions"
      :placeholder="t('globals.terms.unassigned')"
      @select="selectTeam"
      type="team"
      trigger-variant="inline"
      button-class="w-full px-2 py-1 flex justify-center"
    />
  </div>

  <div
    v-else-if="columnId === 'assignee'"
    :class="editableCellClass"
    @click.stop
  >
    <SelectComboBox
      :model-value="assigneeModelValue"
      :items="assigneeOptions"
      :placeholder="t('globals.terms.unassigned')"
      @select="selectAssignee"
      type="user"
      trigger-variant="inline"
      button-class="w-full px-2 py-1"
    />
  </div>

  <div
    v-else-if="columnId === 'tags'"
    ref="tagsCellEl"
    class="flex min-w-0 w-full"
  >
    <div
      v-if="normalizedTags.length"
      ref="tagsListEl"
      class="flex max-h-[4.25rem] min-w-0 w-full flex-wrap content-start gap-1 overflow-hidden py-0.5"
    >
      <TagPill
        v-for="tag in visibleTags"
        :key="tag.key"
        :ref="(el) => setTagItemEl(el, tag.key)"
        :tag="tag"
        size="xs"
        class="max-w-[120px] shrink-0"
      />
      <Tooltip v-if="hiddenTags.length">
        <TooltipTrigger as-child>
          <TagPill
            as="button"
            type="button"
            label="..."
            color="grey"
            size="xs"
            class="shrink-0"
            :aria-label="hiddenTagTitle"
            @click.stop
          />
        </TooltipTrigger>
        <TooltipContent side="top" class="max-w-80">
          <div class="flex max-w-72 flex-wrap gap-1">
            <TagPill
              v-for="tag in hiddenTags"
              :key="`tooltip-${tag.key}`"
              :tag="tag"
              size="xs"
              class="max-w-[140px]"
            />
          </div>
        </TooltipContent>
      </Tooltip>
    </div>
    <span v-else class="text-muted-foreground/60">{{ missing }}</span>
  </div>

  <span
    v-else-if="columnId === 'date_started'"
    class="text-muted-foreground tabular-nums flex justify-center"
    :title="absoluteTitle(conversation.created_at)"
  >
    {{ shortDate(conversation.created_at) }}
  </span>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { format } from 'date-fns'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { useI18n } from 'vue-i18n'
import TagPill from '@shared-ui/components/TagPill.vue'
import StatusPill from '@main/components/status/StatusPill.vue'
import SelectComboBox from '@main/components/combobox/SelectCombobox.vue'
import { useConversationStore } from '@main/stores/conversation'
import { useTeamStore } from '@main/stores/team'
import { useTagStore } from '@main/stores/tag'
import { useUsersStore } from '@main/stores/users'
import { CONVERSATION_DEFAULT_STATUSES } from '@main/constants/conversation'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents'
import { useEmitter } from '@main/composables/useEmitter'
import { formatDateTime } from '@main/utils/date'
import { getStatusDisplayLabel } from '@main/utils/statusVisuals'
import { getConversationDisplayName, hasLinkedCustomer } from '@main/utils/conversationDisplay'
import { Check, Link } from '@qall/icons'

const props = defineProps({
  columnId: { type: String, required: true },
  conversation: { type: Object, required: true },
  now: { type: Date, required: true }
})

const teamStore = useTeamStore()
const tagStore = useTagStore()
const usersStore = useUsersStore()
const conversationStore = useConversationStore()
const emitter = useEmitter()
const { t } = useI18n()
const missing = '—'
const editableCellClass = 'flex min-w-0 rounded-md transition-colors hover:bg-muted/60 focus-within:bg-muted/60'
const tagsCellEl = ref(null)
const tagsListEl = ref(null)
const visibleTagCount = ref(Number.POSITIVE_INFINITY)
let tagsResizeObserver = null
let tagsMeasureFrame = 0
let tagsCellWidth = 0
const tagItemEls = new Map()

const stringValue = (value) => {
  if (value == null) return ''
  if (typeof value === 'string') return value
  return String(value)
}

const findOptionByLabel = (items, label) => {
  const normalizedLabel = stringValue(label).toLowerCase()
  return items.find((item) => stringValue(item.label).toLowerCase() === normalizedLabel) || null
}

const sameValue = (a, b) => String(a ?? '') === String(b ?? '')

const initials = (name) => {
  const parts = stringValue(name).trim().split(/\s+/).filter(Boolean)
  if (!parts.length) return '?'
  return parts.slice(0, 2).map((p) => p[0]).join('').toUpperCase()
}

const stripHTML = (value) => stringValue(value).replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()

// Contact
const contact = computed(() => props.conversation.contact || {})
const displayName = computed(() => getConversationDisplayName(props.conversation))
const contactAvatar = computed(() => stringValue(contact.value.avatar_url))
const contactInitials = computed(() => initials(displayName.value || contact.value.first_name))
const contactEmail = computed(() => stringValue(contact.value.email))
const contactTitle = computed(() =>
  [displayName.value, stringValue(contact.value.email)].filter(Boolean).join(' · ')
)
const isLinked = computed(() => hasLinkedCustomer(props.conversation))

// Subject + activity preview (both shown in the subject column, stacked)
const subjectText = computed(() => stringValue(props.conversation.subject))
const activityText = computed(() =>
  stripHTML(props.conversation.last_interaction || props.conversation.last_message)
)

// Timestamps
const lastActivityAt = computed(() =>
  props.conversation.last_message_at
)

// Priority / status rows receive display names from websocket updates, while
// the picker needs IDs. Resolve the model value from the latest label first.
const priorityName = computed(() => stringValue(props.conversation.priority))
const priorityOptions = computed(() => conversationStore.priorityOptions)
const priorityModelValue = computed(() =>
  findOptionByLabel(priorityOptions.value, priorityName.value)?.value || props.conversation.priority_id || ''
)

// Status
const statusName = computed(() => stringValue(props.conversation.status))
const statusOptions = computed(() => conversationStore.statusOptions)
const currentStatusOption = computed(() =>
  findOptionByLabel(statusOptions.value, statusName.value) || {
    label: statusName.value
  }
)
const statusDisplayLabel = (status) => getStatusDisplayLabel(status, t)
const isSelectedStatus = (status) => sameValue(status?.label, statusName.value)

// Team / assignee mirror the canonical conversation payload: IDs on the row,
// names resolved from app-level stores that are already loaded by App.vue.
const teamOptions = computed(() => [
  { value: 'none', label: t('globals.terms.unassigned') },
  ...teamStore.options
])
const teamModelValue = computed(() =>
  props.conversation.assigned_team_id == null ? 'none' : String(props.conversation.assigned_team_id)
)

const assigneeOptions = computed(() => [
  { value: 'none', label: t('globals.terms.unassigned') },
  ...usersStore.options
])
const assigneeModelValue = computed(() =>
  props.conversation.assigned_user_id == null ? 'none' : String(props.conversation.assigned_user_id)
)

// Tags
const tagsByID = computed(() =>
  new Map(tagStore.tags.map((tag) => [String(tag.id), tag]))
)
const tagsByName = computed(() =>
  new Map(tagStore.tags.map((tag) => [stringValue(tag.name).toLowerCase(), tag]))
)
const normalizedTags = computed(() => {
  const tags = Array.isArray(props.conversation.tags) ? props.conversation.tags : []
  const tagIDs = Array.isArray(props.conversation.tag_ids) ? props.conversation.tag_ids : []
  return tags
    .map((tag, index) => {
      const name = typeof tag === 'string' ? tag : tag?.name
      const id = tagIDs[index] ?? tag?.id
      const fullTag = (id != null ? tagsByID.value.get(String(id)) : null) ||
        tagsByName.value.get(stringValue(name).toLowerCase())

      return {
        key: String(id ?? fullTag?.id ?? name),
        name: stringValue(fullTag?.name || name),
        color: stringValue(fullTag?.color || tag?.color || 'grey')
      }
    })
    .filter((tag) => tag.name)
})
const normalizedVisibleTagCount = computed(() =>
  Math.min(visibleTagCount.value, normalizedTags.value.length)
)
const visibleTags = computed(() => normalizedTags.value.slice(0, normalizedVisibleTagCount.value))
const hiddenTags = computed(() => normalizedTags.value.slice(normalizedVisibleTagCount.value))
const hiddenTagTitle = computed(() => hiddenTags.value.map((tag) => tag.name).join(', '))

onMounted(() => {
  if (props.columnId !== 'tags') return
  if (typeof ResizeObserver !== 'undefined') {
    tagsResizeObserver = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect?.width || 0
      if (Math.abs(width - tagsCellWidth) < 1) return
      tagsCellWidth = width
      scheduleTagsMeasurement()
    })
    if (tagsCellEl.value) {
      tagsResizeObserver.observe(tagsCellEl.value)
    }
  }
  scheduleTagsMeasurement()
})

onBeforeUnmount(() => {
  tagsResizeObserver?.disconnect()
  if (tagsMeasureFrame) {
    window.cancelAnimationFrame(tagsMeasureFrame)
  }
})

watch(tagsCellEl, (el, previousEl) => {
  if (props.columnId !== 'tags') return
  if (previousEl) {
    tagsResizeObserver?.unobserve(previousEl)
  }
  if (el) {
    tagsResizeObserver?.observe(el)
  }
  scheduleTagsMeasurement()
})

watch(normalizedTags, () => {
  if (props.columnId !== 'tags') return
  scheduleTagsMeasurement()
}, { deep: true })

// Time helpers
function compactRelative(value) {
  if (!value) return missing
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return missing

  const diff = Math.abs(date.getTime() - props.now.getTime())
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour

  if (diff >= day) return `${Math.floor(diff / day)}d`
  if (diff >= hour) return `${Math.floor(diff / hour)}h`
  if (diff >= minute) return `${Math.max(1, Math.floor(diff / minute))}m`
  return 'now'
}

function absoluteTitle(value) {
  if (!value) return ''
  return formatDateTime(value)
}

function shortDate(value) {
  if (!value) return missing
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return missing
  return format(date, 'MMM d')
}

function setTagItemEl(el, key) {
  const element = el?.$el || el
  if (element) {
    tagItemEls.set(key, element)
  } else {
    tagItemEls.delete(key)
  }
}

function scheduleTagsMeasurement() {
  if (tagsMeasureFrame) {
    window.cancelAnimationFrame(tagsMeasureFrame)
  }
  tagsMeasureFrame = window.requestAnimationFrame(async () => {
    tagsMeasureFrame = 0
    visibleTagCount.value = normalizedTags.value.length
    await nextTick()
    await updateVisibleTags()
  })
}

async function updateVisibleTags() {
  const el = tagsListEl.value
  if (!el) {
    visibleTagCount.value = normalizedTags.value.length
    return
  }
  const maxBottom = el.getBoundingClientRect().top + el.clientHeight + 1
  const firstHiddenIndex = normalizedTags.value.findIndex((tag) => {
    const tagEl = tagItemEls.get(tag.key)
    if (!tagEl) return false
    return tagEl.getBoundingClientRect().bottom > maxBottom
  })

  if (firstHiddenIndex === -1) {
    visibleTagCount.value = normalizedTags.value.length
    return
  }

  visibleTagCount.value = Math.max(0, firstHiddenIndex - 1)
  for (let attempt = 0; attempt < 3; attempt++) {
    await nextTick()
    if (el.scrollHeight <= el.clientHeight + 1 || visibleTagCount.value <= 0) return
    visibleTagCount.value--
  }
}

function selectTeam(team) {
  const uuid = props.conversation.uuid
  if (!uuid) return
  if (team.value === 'none') {
    if (props.conversation.assigned_team_id == null) return
    conversationStore.removeAssignee('team', uuid)
    return
  }
  if (sameValue(team.value, props.conversation.assigned_team_id)) return
  conversationStore.updateAssignee('team', { assignee_id: parseInt(team.value) }, uuid)
}

function selectAssignee(agent) {
  const uuid = props.conversation.uuid
  if (!uuid) return
  if (agent.value === 'none') {
    if (props.conversation.assigned_user_id == null) return
    conversationStore.removeAssignee('user', uuid)
    return
  }
  if (sameValue(agent.value, props.conversation.assigned_user_id)) return
  conversationStore.updateAssignee('user', { assignee_id: parseInt(agent.value) }, uuid)
}

function selectPriority(priority) {
  if (!props.conversation.uuid || sameValue(priority.label, priorityName.value)) return
  conversationStore.updatePriority(priority.label, props.conversation.uuid)
}

function selectStatus(status) {
  if (!props.conversation.uuid || sameValue(status.label, statusName.value)) return
  if (status.label === CONVERSATION_DEFAULT_STATUSES.SNOOZED) {
    emitter.emit(EMITTER_EVENTS.SET_NESTED_COMMAND, {
      command: 'snooze',
      open: true,
      conversationUUID: props.conversation.uuid
    })
    return
  }
  conversationStore.updateStatus(status.label, props.conversation.uuid)
}
</script>
