<!--
  AgentEditSidebar — left rail of the Agent editor layout.

  Built from the shared @shared-ui/components/ui/sidebar primitives (same
  as every other sidebar in the app, including AiSidebar's non-edit
  branch) so it inherits the platform chroma, active/hover states, and
  spacing automatically rather than re-implementing them. Do not swap
  these back for hand-rolled <nav>/<ul> markup — that's exactly the drift
  this rewrite removed.

  Two grouped sections:
    CONFIGURE → Identity in create mode; all edit tabs once saved
    OPERATE   → History in edit mode

  Active state from `route.name` (not path) — child routes are named
  edit-ai-agent-{identity,instructions,…} or agent-create-identity for
  direct comparison.

  Reads the live agent record from useAgentEdit via inject so this rail
  and the rest of the layout share state — name edits in the Identity
  tab are reflected here immediately.
-->
<template>
  <div class="flex flex-col h-full">
    <!-- Back link + agent name -->
    <SidebarHeader class="border-b border-sidebar-border">
      <div class="flex items-center gap-3 px-1">
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
    </SidebarHeader>

    <!-- Scrollable groups -->
    <SidebarContent>
      <SidebarGroup v-for="group in groups" :key="group.label">
        <div class="px-3 py-1.5 text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
          {{ group.label }}
        </div>
        <SidebarMenu>
          <SidebarMenuItem v-for="item in group.items" :key="item.key">
            <SidebarMenuButton :is-active="isActive(item)" as-child>
              <router-link :to="routeTarget(item)">
                <component :is="item.icon" />
                <span>{{ t(item.labelKey) }}</span>
              </router-link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroup>
    </SidebarContent>
  </div>
</template>

<script setup>
import { computed, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import {
  SidebarContent,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem
} from '@shared-ui/components/ui/sidebar'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { Comment, Wrench, AddressCard, BookOpen, ChevronLeft, Filter, ClockRotateLeft } from "@qall/icons"

const { t } = useI18n()
const route = useRoute()

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

// Configure / Operate group items. Each carries the i18n key, the route
// name to navigate to, and the icon component.
const configureItems = computed(() => {
  const identity = { key: 'identity', labelKey: 'admin.ai.agent.tab.identity', icon: AddressCard, routeName: props.isCreateMode ? 'agent-create-identity' : 'edit-ai-agent-identity' }

  if (props.isCreateMode) return [identity]

  return [
    identity,
    { key: 'detection',    labelKey: 'admin.ai.agent.tab.detection',    icon: Filter,         routeName: 'edit-ai-agent-detection' },
    { key: 'instructions', labelKey: 'admin.ai.agent.tab.instructions', icon: Comment,        routeName: 'edit-ai-agent-instructions' },
    { key: 'tools',        labelKey: 'admin.ai.agent.tab.tools',        icon: Wrench,         routeName: 'edit-ai-agent-tools' },
    { key: 'knowledge',    labelKey: 'admin.ai.agent.tab.knowledge',    icon: BookOpen,       routeName: 'edit-ai-agent-knowledge' },
  ]
})

const operateItems = computed(() =>
  props.isCreateMode
    ? []
    : [{ key: 'history', labelKey: 'admin.ai.agent.tab.history', icon: ClockRotateLeft, routeName: 'edit-ai-agent-history' }]
)

// Grouped sections for the template. Operate is omitted when empty
// (create mode) so no empty label renders.
const groups = computed(() => {
  const out = [{ label: 'Configure', items: configureItems.value }]
  if (operateItems.value.length) {
    out.push({ label: 'Operate', items: operateItems.value })
  }
  return out
})

function isActive(item) {
  return route.name === item.routeName
}

function routeTarget(item) {
  if (!props.agentId) return { name: item.routeName }
  return { name: item.routeName, params: { id: props.agentId } }
}
</script>
