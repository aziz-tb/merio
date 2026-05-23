<template>
  <div class="flex flex-col h-full">
    <Tabs default-value="details" class="flex flex-col h-full">
      <!-- Tab bar -->
      <div class="flex items-center px-8 border-b border-border h-14 shrink-0">
        <TabsList class="bg-transparent h-14 rounded-none  p-0 gap-5">
          <TabsTrigger
            value="details"
            class="bg-transparent rounded-none px-0 h-14 shadow-none border-b-2 border-transparent data-[state=active]:bg-transparent data-[state=active]:shadow-none data-[state=active]:border-amber-500 data-[state=active]:text-foreground text-muted-foreground"
          >
            Details
          </TabsTrigger>
          <TabsTrigger
            value="composer"
            class="bg-transparent rounded-none px-0 h-14 shadow-none border-b-2 border-transparent data-[state=active]:bg-transparent data-[state=active]:shadow-none data-[state=active]:border-amber-500 data-[state=active]:text-foreground text-muted-foreground"
          >
            {{ $t('composer.tabTitle') }}
          </TabsTrigger>
        </TabsList>
      </div>

      <!-- Details tab -->
      <TabsContent value="details" class="flex-1 overflow-y-auto m-0 p-4 space-y-3">
        <!-- Contact mini-card -->
        <ConversationSideBarContact class="pb-2" />

        <!-- Actions: agent, team, priority, tags -->
        <Collapsible defaultOpen class="group/section border-t border-border pt-3">
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('globals.terms.action', 2) }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3 space-y-3">
            <SelectComboBox
              v-model="conversationStore.current.assigned_user_id"
              :items="[{ value: 'none', label: t('globals.terms.none') }, ...usersStore.options]"
              :placeholder="t('placeholders.selectAgent')"
              @select="selectAgent"
              type="user"
            />
            <SelectComboBox
              v-model="conversationStore.current.assigned_team_id"
              :items="[{ value: 'none', label: t('globals.terms.none') }, ...teamsStore.options]"
              :placeholder="t('placeholders.selectTeam')"
              @select="selectTeam"
              type="team"
            />
            <SelectComboBox
              v-model="conversationStore.current.priority_id"
              :items="priorityOptions"
              :placeholder="t('placeholders.selectPriority')"
              @select="selectPriority"
              type="priority"
            />
            
            <div class="relative">
              <SelectTag
                v-if="conversationStore.current"
                v-model="conversationStore.current.tags"
                :items="tagOptions"
                :placeholder="t('placeholders.selectTags')"
              />

              <Button
                v-if="conversationStore.current?.uuid"
                variant="ghost"
                size="icon"
                class="absolute right-1 top-1 z-10 size-7 bg-background/80"
                :disabled="reloading"
                :title="t('tag.reload.tooltip')"
                @click.stop="triggerReload"
              >
                <RefreshCw color="gray" :class="{ 'animate-spin': reloading }" />
              </Button>
            </div>
          </CollapsibleContent>
        </Collapsible>

        <!-- Information -->
        <Collapsible defaultOpen class="group/section border-t border-border pt-3">
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('conversation.sidebar.information') }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3">
            <ConversationInfo />
          </CollapsibleContent>
        </Collapsible>

        <!-- Contact attributes -->
        <Collapsible
          v-if="customAttributeStore.contactAttributeOptions.length > 0"
          defaultOpen
          class="group/section border-t border-border pt-3"
        >
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('conversation.sidebar.contactAttributes') }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3">
            <CustomAttributes
              :loading="conversationStore.current.loading"
              :attributes="customAttributeStore.contactAttributeOptions"
              :customAttributes="conversationStore.current?.contact?.custom_attributes || {}"
              @update:setattributes="updateContactCustomAttributes"
            />
          </CollapsibleContent>
        </Collapsible>

        <!-- Page visits (livechat only) -->
        <Collapsible
          v-if="conversationStore.current?.inbox_channel === 'livechat'"
          class="group/section border-t border-border pt-3"
        >
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('conversation.sidebar.lastVisitedPages') }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3">
            <ConversationSideBarPageVisits />
          </CollapsibleContent>
        </Collapsible>

        <!-- Contact notes -->
        <Collapsible
          v-if="conversationStore.current?.contact?.id && userStore.can('contact_notes:read')"
          class="group/section border-t border-border pt-3"
        >
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('globals.terms.note', 2) }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3">
            <ContactNotes :contact-id="conversationStore.current.contact.id" compact />
          </CollapsibleContent>
        </Collapsible>

        <!-- Previous conversations -->
        <Collapsible class="group/section border-t border-border pt-3">
          <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
            <span>{{ t('conversation.sidebar.previousConvo') }}</span>
            <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="pt-3">
            <PreviousConversations />
          </CollapsibleContent>
        </Collapsible>
      </TabsContent>

      <!-- Composer tab — AI agents the operator can run on this ticket.
           Body fetches latest runs + enabled agents on mount; see
           ComposerTab.vue for the action flow (Run / Send / Edit /
           Regenerate). -->
      <TabsContent value="composer" class="flex-1 overflow-y-auto m-0">
        <ComposerTab v-if="conversationUUID" :uuid="conversationUUID" />
      </TabsContent>
    </Tabs>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'
import { useConversationStore } from '@/stores/conversation'
import { useUsersStore } from '@/stores/users'
import { useTeamStore } from '@/stores/team'
import { useTagStore } from '@/stores/tag'
import { useUserStore } from '@/stores/user'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@shared-ui/components/ui/tabs'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@shared-ui/components/ui/collapsible'
import { ChevronDown, RefreshCw } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import ConversationInfo from './ConversationInfo.vue'
import ConversationSideBarContact from '@/features/conversation/sidebar/ConversationSideBarContact.vue'
import { SelectTag } from '@shared-ui/components/ui/select'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { useEmitter } from '../../../composables/useEmitter'
import { useI18n } from 'vue-i18n'
import CustomAttributes from '@/features/conversation/sidebar/CustomAttributes.vue'
import { useCustomAttributeStore } from '../../../stores/customAttributes'
import ContactNotes from '@/features/contact/ContactNotes.vue'
import PreviousConversations from '@/features/conversation/sidebar/PreviousConversations.vue'
import ConversationSideBarPageVisits from '@/features/conversation/sidebar/ConversationSideBarPageVisits.vue'
import ComposerTab from '@/features/conversation/sidebar/ComposerTab.vue'
import SelectComboBox from '@main/components/combobox/SelectCombobox.vue'
import api from '../../../api'

const customAttributeStore = useCustomAttributeStore()
const emitter = useEmitter()
const conversationStore = useConversationStore()
const usersStore = useUsersStore()
const teamsStore = useTeamStore()
const tagStore = useTagStore()
const userStore = useUserStore()
const tags = ref([])
const reloadingUUID = ref('')
const { t } = useI18n()
let isConversationChange = false
let reloadTimeout = null
const reloadTimeoutMs = 2 * 60 * 1000
customAttributeStore.fetchCustomAttributes()

// Watch for changes in the current conversation and set the flag
watch(
  () => conversationStore.current,
  (newConversation, oldConversation) => {
    // Set the flag when the conversation changes
    if (newConversation?.uuid !== oldConversation?.uuid) {
      isConversationChange = true
    }
  },
  { immediate: true }
)

onMounted(async () => {
  await fetchTags()
  emitter.on(EMITTER_EVENTS.TAG_CLASSIFICATION_UPDATE, handleTagClassificationUpdate)
})

onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.TAG_CLASSIFICATION_UPDATE, handleTagClassificationUpdate)
  clearReloadTimeout()
})

// Watch for changes in the tags and upsert the tags
watch(
  () => conversationStore.current?.tags,
  (newTags, oldTags) => {
    // Skip if the tags change is due to a conversation change.
    if (isConversationChange) {
      isConversationChange = false
      return
    }

    // Skip if the tags are the same (deep comparison)
    if (
      Array.isArray(newTags) &&
      Array.isArray(oldTags) &&
      newTags.length === oldTags.length &&
      newTags.every((item) => oldTags.includes(item))
    ) {
      return
    }

    conversationStore.upsertTags({
      tags: newTags
    })
  },
  { immediate: false }
)

const priorityOptions = computed(() => conversationStore.priorityOptions)
const tagOptions = computed(() => {
  return tagStore.tags.map((tag) => ({
    label: tag.name,
    value: tag.name,
    color: tag.color || 'grey'
  }))
})

// Composer needs the conversation UUID to fetch / persist runs. Tracked
// as a computed so the tab re-fetches when the page swaps tickets.
const conversationUUID = computed(() => conversationStore.current?.uuid || '')
const reloading = computed(() => reloadingUUID.value === conversationUUID.value)

const fetchTags = async () => {
  await tagStore.fetchTags()
  tags.value = tagStore.tags.map((item) => item.name)
}

const triggerReload = async () => {
  const uuid = conversationStore.current?.uuid
  if (!uuid) return
  startReloading(uuid)
  try {
    await api.classifyConversationTags(uuid)
  } catch (error) {
    const status = error?.response?.status
    if (status === 409) {
      return
    }
    stopReloading(uuid)
    let description = handleHTTPError(error).message
    if (status === 503) description = t('tag.reload.notAvailable')
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description
    })
  }
}

const startReloading = (uuid) => {
  reloadingUUID.value = uuid
  clearReloadTimeout()
  reloadTimeout = window.setTimeout(() => {
    if (reloadingUUID.value !== uuid) return
    stopReloading(uuid)
    if (conversationStore.current?.uuid === uuid) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('tag.reload.timeout')
      })
    }
  }, reloadTimeoutMs)
}

const stopReloading = (uuid) => {
  if (uuid && reloadingUUID.value !== uuid) return
  reloadingUUID.value = ''
  clearReloadTimeout()
}

const clearReloadTimeout = () => {
  if (!reloadTimeout) return
  window.clearTimeout(reloadTimeout)
  reloadTimeout = null
}

const handleTagClassificationUpdate = (update) => {
  if (!update?.uuid || update.uuid !== reloadingUUID.value) return
  stopReloading(update.uuid)
  if (update.tag_classification?.status === 'failed') {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('tag.reload.failed')
    })
  }
}

const handleAssignedUserChange = (id) => {
  conversationStore.updateAssignee('user', {
    assignee_id: parseInt(id)
  })
}

const handleAssignedTeamChange = (id) => {
  conversationStore.updateAssignee('team', {
    assignee_id: parseInt(id)
  })
}

const handleRemoveAssignee = (type) => {
  conversationStore.removeAssignee(type)
}

const handlePriorityChange = (priority) => {
  conversationStore.updatePriority(priority)
}

const selectAgent = (agent) => {
  if (agent.value === 'none') {
    handleRemoveAssignee('user')
    return
  }
  conversationStore.current.assigned_user_id = agent.value
  handleAssignedUserChange(agent.value)
}

const selectTeam = (team) => {
  if (team.value === 'none') {
    handleRemoveAssignee('team')
    return
  }
  handleAssignedTeamChange(team.value)
}

const selectPriority = (priority) => {
  conversationStore.current.priority = priority.label
  conversationStore.current.priority_id = priority.value
  handlePriorityChange(priority.label)
}

const updateContactCustomAttributes = async (attributes) => {
  let previousAttributes = conversationStore.current.contact.custom_attributes
  try {
    conversationStore.current.contact.custom_attributes = attributes
    await api.updateContactCustomAttribute(conversationStore.current.uuid, attributes)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
    conversationStore.current.contact.custom_attributes = previousAttributes
  }
}
</script>
