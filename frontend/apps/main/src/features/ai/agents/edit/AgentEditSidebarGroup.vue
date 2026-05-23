<!--
  SidebarGroup — one Configure/Operate section inside AgentEditSidebar.

  Renders an uppercase muted group label followed by router-link rows.
  Active state when the current route name matches the row's
  routeName. Active row gets bg-background-secondary; inactive rows
  inherit muted text + hover.

  Lifted into its own component so AgentEditSidebar's template stays
  scannable — each group is a one-liner consuming items.
-->
<template>
  <div class="px-2 mb-3">
    <div class="px-2 py-1.5 text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
      {{ label }}
    </div>
    <ul class="space-y-0.5">
      <li v-for="item in items" :key="item.key">
        <router-link
          :to="{ name: item.routeName, params: { id: agentId } }"
          :class="[
            'flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-colors',
            isActive(item) ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-accent/50',
          ]"
        >
          <component :is="item.icon" class="w-4 h-4 shrink-0" />
          <span class="flex-1 truncate">{{ t(item.labelKey) }}</span>
        </router-link>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

defineProps({
  label:   { type: String, required: true },
  items:   { type: Array,  required: true },
  agentId: { type: Number, required: true },
})

const route = useRoute()
const { t } = useI18n()

function isActive(item) {
  return route.name === item.routeName
}
</script>
