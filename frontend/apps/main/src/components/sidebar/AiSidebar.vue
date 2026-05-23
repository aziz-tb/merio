<!--
  AI-mode contents for the secondary sidebar. v1 renders a flat list of
  two items (Agents / Tools). When a third sub-area lands (presets, eval,
  audit trace, etc.) regroup into Configure / Operate / Quality / Analytics
  collapsibles per the F9 navigation proposal.
-->
<template>
  <AgentEditSidebar v-if="isAgentEditRoute" :agent-id="agentId" />

  <div v-else class="flex flex-col h-full">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <div class="px-1">
            <span class="text-lg font-semibold text-foreground px-2">
              {{ t('globals.terms.ai') }}
            </span>
          </div>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
    <SidebarContent>
      <SidebarGroup>
        <SidebarMenu>
          <SidebarMenuItem v-for="item in filteredAiNavItems" :key="item.titleKey">
            <SidebarMenuButton :isActive="isActive(item.href)" asChild>
              <router-link :to="item.href">
                <component :is="navIconMap[item.icon]" v-if="item.icon" />
                <span>{{ t(item.titleKey, item.isTitleKeyPlural === true ? 2 : 1) }}</span>
              </router-link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroup>
    </SidebarContent>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Bot, Wrench } from 'lucide-vue-next'
import {
  SidebarContent,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem
} from '@shared-ui/components/ui/sidebar'
import { aiNavItems } from '@/constants/navigation'
import { useUserStore } from '@main/stores/user'
import AgentEditSidebar from '@main/features/ai/agents/edit/AgentEditSidebar.vue'

const { t } = useI18n()
const route = useRoute()
const userStore = useUserStore()

const navIconMap = { Bot, Wrench }
const agentId = computed(() => Number(route.params.id))
const isAgentEditRoute = computed(() =>
  route.matched.some((record) => record.meta?.layoutContext === 'ai-agent-edit')
)

const filteredAiNavItems = computed(() =>
  aiNavItems.filter((item) => !item.permission || userStore.can(item.permission))
)

const isActive = (href) => route.path.startsWith(href)
</script>
