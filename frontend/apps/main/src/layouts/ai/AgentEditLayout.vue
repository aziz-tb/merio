<!--
  AgentEditLayout — editor shell for editing an AI agent.

  The left navigation now lives in the shared AI sidebar. This layout
  owns only the editor card and the optional preview rail. The route-
  scoped edit/preview state is provided above both the sidebar and this
  layout by RouteContextHost, so either side can inject the same
  reactive instance.

  The middle panel's default-size is reactive to preview.isOpen so
  the main column expands to fill the gap when the preview rail is
  closed.

  Save model:
    - one dirty state, one Save button (in AgentEditHeader)
    - tab switches + route exits prompt when dirty
    - browser close prompt via beforeunload (registered in the composable)
    - Cmd+S / Ctrl+S triggers Save (registered in the composable)

  Vue Router 4 note: onBeforeRouteUpdate fires on the parent layout
  for child-route changes (tab switches) when the parent component
  stays mounted, which is our case. onBeforeRouteLeave covers exits
  out of the layout entirely. A global router.beforeEach was tempting
  as belt-and-braces but would double-prompt — the per-component
  guards already cover the surface. If those ever fail to fire,
  fallback is a single router.beforeEach (the per-component guards
  must be removed when adding it to avoid double-prompts).
-->
<template>
  <ResizablePanelGroup direction="horizontal" class="h-full w-full">
    <!-- Middle: main content card — header + active tab body. -->
    <ResizablePanel :default-size="preview.isOpen.value ? 74 : 100" :min-size="40">
      <div
        :class="[
          'h-full bg-background border border-input shadow-sm overflow-hidden rounded-lg flex flex-col',
          preview.isOpen.value ? 'mr-0.5' : ''
        ]"
      >
        <AgentEditHeader
          :preview-open="preview.isOpen.value"
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
      v-if="preview.isOpen.value"
      disabled
      class="opacity-0 pointer-events-none"
    />

    <!-- Right: preview rail (conditional). State (linkedTicket,
         composeBody, lastRun, …) lives in useAgentPreview via the
         route-scoped provider above this layout, so it survives child
         route changes and stays in sync with the shared sidebar. -->
    <ResizablePanel
      v-if="preview.isOpen.value"
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
import { inject } from 'vue'
import {
  onBeforeRouteLeave,
  onBeforeRouteUpdate,
} from 'vue-router'
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

// Tab-switch guard: fires when the matched parent stays the same and
// only a child route changes. Vue Router 4 fires this on the parent
// layout's component, which is exactly here. Returning a boolean is
// the modern shape (avoids the legacy `next` callback).
onBeforeRouteUpdate(() => edit.confirmDiscardIfDirty())

// Layout-exit guard: fires when navigation leaves /ai/agents/:id/edit/*
// entirely (e.g., the "← Agents" link in the sidebar).
onBeforeRouteLeave(() => edit.confirmDiscardIfDirty())
</script>
