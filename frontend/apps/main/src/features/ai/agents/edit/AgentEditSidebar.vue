<!--
  AgentEditSidebar — left rail of the Edit Agent layout.

  Two grouped sections:
    CONFIGURE → Identity, Instructions, Tools, Knowledge, Detection
    OPERATE   → History

  Active state from `route.name` (not path) — child routes are named
  edit-ai-agent-{identity,instructions,…} for direct comparison.

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
        <span  class="text-md font-semibold text-foreground truncate">{{ agent?.name || '…' }}</span>
      <div>
        
      </div>
    </div>

    <!-- Scrollable groups -->
    <div class="flex-1 overflow-auto py-2">
      <SidebarGroup label="Configure" :items="configureItems" :agent-id="agentId" />
      <SidebarGroup label="Operate" :items="operateItems" :agent-id="agentId" />
    </div>
  </nav>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ChevronLeft,
  IdCard,
  MessageCircle,
  Wrench,
  BookOpen,
  Filter,
  History,
} from 'lucide-vue-next'
import SidebarGroup from './AgentEditSidebarGroup.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'

defineProps({
  agentId: { type: Number, required: true },
})

const { t } = useI18n()

// Live agent record from useAgentEdit (provided by AgentEditLayout).
// .value because useAgentEdit returns { agent: ref, ... }.
const edit = inject(AGENT_EDIT_KEY)
const agent = computed(() => edit?.agent.value)

// Configure / Operate group items. Each carries the i18n key, the
// route name to navigate to, the lucide icon component, and an
// optional badge slot (e.g., tool count) that the group renders.
const configureItems = computed(() => [
  { key: 'identity',     labelKey: 'admin.ai.agent.tab.identity',     icon: IdCard,        routeName: 'edit-ai-agent-identity' },
  { key: 'instructions', labelKey: 'admin.ai.agent.tab.instructions', icon: MessageCircle, routeName: 'edit-ai-agent-instructions' },
  { key: 'tools',        labelKey: 'admin.ai.agent.tab.tools',        icon: Wrench,        routeName: 'edit-ai-agent-tools' },
  { key: 'knowledge',    labelKey: 'admin.ai.agent.tab.knowledge',    icon: BookOpen,      routeName: 'edit-ai-agent-knowledge' },
  { key: 'detection',   labelKey: 'admin.ai.agent.tab.detection',   icon: Filter,        routeName: 'edit-ai-agent-detection' },
])

const operateItems = computed(() => [
  { key: 'history', labelKey: 'admin.ai.agent.tab.history', icon: History, routeName: 'edit-ai-agent-history' },
])
</script>