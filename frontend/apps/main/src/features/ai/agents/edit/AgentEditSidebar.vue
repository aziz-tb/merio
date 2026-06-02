<!--
  AgentEditSidebar — left rail of the Agent editor layout.

  Two grouped sections:
    CONFIGURE → Identity in create mode; all edit tabs once saved
    OPERATE   → History in edit mode

  Active state from `route.name` (not path) — child routes are named
  edit-ai-agent-{identity,instructions,…} or agent-create-identity for
  direct comparison.

  Header: back link to /ai/agents, agent name. The slug / enabled
  subtitle was removed — slug is a developer-facing field surfaced
  in the Identity tab, and enabled state shows up next to the toggle
  in the same place.

  Reads the live agent record from useAgentEdit via inject so this
  rail and the rest of the layout share state — name edits in the
  Identity tab are reflected here immediately.
-->
<template>
  <nav class="w-full shrink-0 flex flex-col h-full overflow-hidden bg-background">
    <!-- Back link + agent header -->
    <div class="p-4 border-b border-input inline-flex items-center gap-3 shrink-0">
      <router-link
        :to="{ name: 'ai-agent-list' }"
        class="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ChevronLeft class="w-3.5 h-3.5" />
      </router-link>
      <span class="text-md font-semibold text-foreground truncate">
        {{ sidebarTitle }}
      </span>
    </div>

    <!-- Scrollable groups -->
    <div class="flex-1 overflow-auto py-2">
      <SidebarGroup label="Configure" :items="configureItems" :agent-id="props.agentId" />
      <SidebarGroup
        v-if="operateItems.length"
        label="Operate"
        :items="operateItems"
        :agent-id="props.agentId"
      />
    </div>
  </nav>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import SidebarGroup from './AgentEditSidebarGroup.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { Comment, Wrench, AddressCard, BookOpen, ChevronLeft, Filter, ClockRotateLeft } from "@qall/icons"
const { t } = useI18n()

// Live agent record from useAgentEdit (provided by AgentEditLayout).
// .value because useAgentEdit returns { agent: ref, ... }.
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)
const props = defineProps({
  agentId: { type: Number, default: null },
  isCreateMode: { type: Boolean, default: false },
})
const sidebarTitle = computed(() =>
  props.isCreateMode ? t('admin.ai.agent.newAgent') : agent.value?.name || '…'
)

// Configure / Operate group items. Each carries the i18n key, the
// route name to navigate to, the lucide icon component, and an
// optional badge slot (e.g., tool count) that the group renders.
const configureItems = computed(() => {
  const identity = { key: 'identity', labelKey: 'admin.ai.agent.tab.identity', icon: AddressCard, routeName: props.isCreateMode ? 'agent-create-identity' : 'edit-ai-agent-identity' }

  if (props.isCreateMode) return [identity]

  return [
    identity,
    { key: 'instructions', labelKey: 'admin.ai.agent.tab.instructions', icon: Comment, routeName: 'edit-ai-agent-instructions' },
    { key: 'tools',        labelKey: 'admin.ai.agent.tab.tools',        icon: Wrench,        routeName: 'edit-ai-agent-tools' },
    { key: 'knowledge',    labelKey: 'admin.ai.agent.tab.knowledge',    icon: BookOpen,      routeName: 'edit-ai-agent-knowledge' },
    { key: 'detection',    labelKey: 'admin.ai.agent.tab.detection',    icon: Filter,        routeName: 'edit-ai-agent-detection' },
  ]
})

const operateItems = computed(() =>
  props.isCreateMode
    ? []
    : [{ key: 'history', labelKey: 'admin.ai.agent.tab.history', icon: ClockRotateLeft, routeName: 'edit-ai-agent-history' }]
)
</script>
