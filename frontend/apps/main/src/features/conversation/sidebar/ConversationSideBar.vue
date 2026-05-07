<template>
  <div class="flex flex-col h-full">
    <Tabs default-value="details" class="flex flex-col h-full">
      <!-- Tab bar -->
      <div class="flex items-center px-4 border-b border-border h-12 shrink-0">
        <TabsList class="bg-transparent h-12 rounded-none p-0 gap-5">
          <TabsTrigger
            value="details"
            class="bg-transparent rounded-none px-0 h-12 shadow-none border-b-2 border-transparent data-[state=active]:bg-transparent data-[state=active]:shadow-none data-[state=active]:border-amber-500 data-[state=active]:text-foreground text-muted-foreground"
          >
            Details
          </TabsTrigger>
          <TabsTrigger
            value="copilot"
            disabled
            title="Coming soon"
            class="bg-transparent rounded-none px-0 h-12 shadow-none border-b-2 border-transparent text-muted-foreground/60"
          >
            Copilot
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
            <SelectTag
              v-if="conversationStore.current"
              v-model="conversationStore.current.tags"
              :items="tags.map((tag) => ({ label: tag, value: tag }))"
              :placeholder="t('placeholders.selectTags')"
            />
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

      <!-- Copilot tab (placeholder) -->
      <TabsContent value="copilot" class="flex-1 overflow-y-auto m-0 p-4 text-center text-sm text-muted-foreground">
        Coming soon
      </TabsContent>
    </Tabs>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, computed } from 'vue'
import { useConversationStore } from '@/stores/conversation'
import { useUsersStore } from '@/stores/users'
import { useTeamStore } from '@/stores/team'
import { useTagStore } from '@/stores/tag'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@shared-ui/components/ui/tabs'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@shared-ui/components/ui/collapsible'
import { ChevronDown } from 'lucide-vue-next'
import ConversationInfo from './ConversationInfo.vue'
import ConversationSideBarContact from '@/features/conversation/sidebar/ConversationSideBarContact.vue'
import { SelectTag } from '@shared-ui/components/ui/select'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { useEmitter } from '../../../composables/useEmitter'
import { useI18n } from 'vue-i18n'
import CustomAttributes from '@/features/conversation/sidebar/CustomAttributes.vue'
import { useCustomAttributeStore } from '../../../stores/customAttributes'
import PreviousConversations from '@/features/conversation/sidebar/PreviousConversations.vue'
import ConversationSideBarPageVisits from '@/features/conversation/sidebar/ConversationSideBarPageVisits.vue'
import SelectComboBox from '@main/components/combobox/SelectCombobox.vue'
import api from '../../../api'

const customAttributeStore = useCustomAttributeStore()
const emitter = useEmitter()
const conversationStore = useConversationStore()
const usersStore = useUsersStore()
const teamsStore = useTeamStore()
const tagStore = useTagStore()
const tags = ref([])
const { t } = useI18n()
let isConversationChange = false
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

const fetchTags = async () => {
  await tagStore.fetchTags()
  tags.value = tagStore.tags.map((item) => item.name)
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
