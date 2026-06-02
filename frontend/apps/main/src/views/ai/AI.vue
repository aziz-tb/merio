<!--
  Top-level AI area. Bare router-view — AI children (AgentList,
  AgentEditLayout, ToolList, CreateOrEditTool) handle their own chrome
  internally (flex flex-col h-full + per-section padding + internal
  overflow-auto), matching the Knowledge area's pattern. No shared help rail.
-->
<template>
  <router-view v-if="isFullBleedRoute" />
  <div v-else class="h-full overflow-y-auto">
    <div class="px-6 py-6 sm:px-8">
      <router-view />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const isFullBleedRoute = computed(() =>
  route.matched.some((record) => record.meta?.fullBleed)
)
</script>
