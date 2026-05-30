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
    - tab switches close the path picker because it targets a condition
      in the previous tab; Preview keeps its own preferred-open state
    - browser close prompt via beforeunload (registered in the composable)
    - Cmd+S / Ctrl+S triggers Save (registered in the composable)

  Vue Router 4 note: tab switches are child-route changes while this
  layout stays mounted. They are intentionally free navigation now, but
  route changes still close transient picker UI. onBeforeRouteLeave covers
  exits out of the layout entirely.
-->
<template>
  <ResizablePanelGroup direction="horizontal" class="h-full w-full">
    <!-- Middle: main content card — header + active tab body. -->
    <ResizablePanel :default-size="isCreateMode || !rail.isOpen.value ? 100 : 70" :min-size="40">
      <div
        :class="[
          'h-full bg-background border border-input shadow-sm overflow-hidden rounded-lg flex flex-col',
          !isCreateMode && rail.isOpen.value ? 'mr-0.5' : ''
        ]"
      >
        <AgentEditHeader
          :preview-open="preview.isOpen.value"
          :is-create-mode="isCreateMode"
          @toggle-preview="preview.toggle()"
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
      v-if="!isCreateMode && rail.isOpen.value"
      disabled
      class="opacity-0 pointer-events-none"
    />

    <!-- Right: single rail slot. Preview and the detection path picker are
         mutually exclusive panels coordinated by useAgentRightRail. -->
    <ResizablePanel
      v-if="!isCreateMode && rail.isOpen.value"
      :default-size="30"
      :min-size="33"
      :max-size="35"
    >
      <div class="h-full bg-background border border-input shadow-sm overflow-hidden rounded-lg ml-0.5">
        <AgentPreviewPanel v-if="rail.isActive('preview')" @close="preview.close()" />
        <PathPickerPanel v-else-if="rail.isActive('path-picker')" />
      </div>
    </ResizablePanel>
  </ResizablePanelGroup>
</template>

<script setup>
import { computed, inject, onBeforeUnmount, onMounted, watch } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@shared-ui/components/ui/resizable'
import { DotLoader } from '@shared-ui/components/ui/loader'
import AgentEditHeader from '@main/features/ai/agents/edit/AgentEditHeader.vue'
import AgentPreviewPanel from '@main/features/ai/agents/preview/AgentPreviewPanel.vue'
import PathPickerPanel from '@main/features/admin/detection/PathPickerPanel.vue'
import { AGENT_EDIT_KEY } from '@/composables/useAgentEdit.js'
import { AGENT_PREVIEW_KEY } from '@/composables/useAgentPreview.js'
import { useAgentRightRail } from '@/composables/useAgentRightRail'
import { usePathPicker } from '@/composables/usePathPicker'

// RouteContextHost provides both contexts above the global sidebar and
// this layout so the edit nav, header, tabs, and preview rail all stay
// in sync off the same reactive state.
const edit = inject(AGENT_EDIT_KEY)
const preview = inject(AGENT_PREVIEW_KEY)
const rail = useAgentRightRail()
const picker = usePathPicker()
const route = useRoute()
const isCreateMode = computed(() => !route.params.id)

const restorePreferredPreview = () => {
  if (!isCreateMode.value && preview.preferOpen.value && !rail.isOpen.value) {
    preview.open()
  }
}

onMounted(restorePreferredPreview)
watch(isCreateMode, restorePreferredPreview)

// Tab switches are route updates while the layout stays mounted. The picker
// is tied to a condition in the current tab, so close it on any route change
// while leaving Preview to its own preferred-open behavior.
watch(() => route.fullPath, (newPath, oldPath) => {
  if (newPath !== oldPath) {
    picker.close()
  }
})

onBeforeUnmount(() => {
  picker.reset()
  rail.reset()
})

// Layout-exit guard: fires when navigation leaves the agent editor
// entirely (e.g., the "← Agents" link in the sidebar).
onBeforeRouteLeave(() => edit.confirmDiscardIfDirty())
</script>
