<!--
  AgentTabChangelog — placeholder. The sidebar entry is disabled, so
  this tab is only reachable via deep-link / direct URL. Renders a
  "coming soon" message describing future contents.

  Not in the active implementation order — exists so a stray
  bookmark doesn't 404.
-->
<template>
  <div class="px-6 py-6 max-w-xl">
    <h2 class="text-lg font-semibold mb-2">{{ t('admin.ai.agent.tab.changelog') }}</h2>
    <p class="text-sm text-muted-foreground mb-4">
      Coming soon. Will record:
    </p>
    <ul class="text-sm text-muted-foreground list-disc pl-5 space-y-1">
      <li>Prompt revisions</li>
      <li>Tool config changes</li>
      <li>Knowledge scope changes</li>
      <li>Activity-log entries scoped to this agent</li>
    </ul>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
