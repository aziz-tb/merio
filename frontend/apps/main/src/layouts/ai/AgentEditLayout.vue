<!--
  AgentEditLayout — editor shell for editing or creating an AI agent.

  The left navigation now lives in the shared AI sidebar. This layout
  owns only the editor card and the optional preview rail. In create
  mode the route exposes only Identity, and the preview rail is hidden
  until the agent has been saved. The route-scoped edit/preview state
  is provided above both the sidebar and this layout by RouteContextHost,
  so either side can inject the same reactive instance.

  The middle panel's default-size is reactive to preview.isOpen so
  the main column expands to fill the gap when the preview rail is
  closed. In create mode it always takes the full width.

  Save model:
    - one dirty state, one Save button (in AgentEditHeader)
    - route exits prompt when dirty; tab switches do not
    - browser close prompt via beforeunload (registered in the composable)
    - Cmd+S / Ctrl+S triggers Save (registered in the composable)

  Vue Router 4 note: tab switches are child-route changes while this
  layout stays mounted. They are intentionally free navigation now.
  onBeforeRouteLeave covers exits out of the layout entirely.
-->
<template>
  <ResizablePanelGroup direction="horizontal" class="h-full w-full">
    <!-- Middle: main content card — header + active tab body. -->
    <ResizablePanel :default-size="isCreateMode || !preview.isOpen.value ? 100 : 74" :min-size="40">
      <div
        :class="[
          'h-full bg-background border border-input shadow-sm overflow-hidden rounded-lg flex flex-col',
          !isCreateMode && preview.isOpen.value ? 'mr-0.5' : ''
        ]"
      >
        <AgentEditHeader
          :preview-open="preview.isOpen.value"
          :is-create-mode="isCreateMode"
          @toggle-preview="preview.isOpen.value = !preview.isOpen.value"
        />
        <div class="flex-1 overflow-auto">
          <router-view v-if="!edit.loading.value && edit.agent.value" />
          <div
            v-else-if="edit.loading.value"
            class="flex items-center justify-center py-12 text-muted-foreground"
          >
            <DotLoader />
          </div>
          <div
            v-else-if="edit.loadError.value"
            class="px-6 py-6 text-sm text-destructive"
          >
            {{ edit.loadError.value }}
          </div>
        </div>
      </div>
    </ResizablePanel>

    <ResizableHandle
      v-if="!isCreateMode && preview.isOpen.value"
      disabled
      class="opacity-0 pointer-events-none"
    />

    <!-- Right: preview rail (conditional). State (linkedTicket,
         composeBody, lastRun, …) lives in useAgentPreview via the
         route-scoped provider above this layout, so it survives child
         route changes and stays in sync with the shared sidebar. -->
    <ResizablePanel
      v-if="!isCreateMode && preview.isOpen.value"
      :default-size="30"
      :min-size="33"
      :max-size="35"
    >
      <div class="h-full bg-background border border-input shadow-sm overflow-hidden rounded-lg ml-0.5">
        <AgentPreviewPanel @close="preview.isOpen.value = false" />
      </div>
    </ResizablePanel>
  </ResizablePanelGroup>
</template>

<script setup>
import { computed, inject } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@shared-ui/components/ui/resizable'
import { DotLoader } from '@shared-ui/components/ui/loader'
import AgentEditHeader from '@main/features/ai/agents/edit/AgentEditHeader.vue'
import AgentPreviewPanel from '@main/features/ai/agents/preview/AgentPreviewPanel.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { AGENT_PREVIEW_KEY } from '@/composables/useAgentPreview.js'

// RouteContextHost provides both contexts above the global sidebar and
// this layout so the edit nav, header, tabs, and preview rail all stay
// in sync off the same reactive state.
const edit = inject(AGENT_EDIT_KEY)
const preview = inject(AGENT_PREVIEW_KEY)
const route = useRoute()
const isCreateMode = computed(() => !route.params.id)

// Layout-exit guard: fires when navigation leaves the agent editor
// entirely (e.g., the "← Agents" link in the sidebar).
onBeforeRouteLeave(() => edit.confirmDiscardIfDirty())
</script>
