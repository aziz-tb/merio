<template>
  <PageHeader :description="t('admin.ai.tool.help')">
    <template #actions>
      <RouterLink :to="{ name: 'new-ai-tool' }">
        <Button>{{ $t('aiTool.new') }}</Button>
      </RouterLink>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="tools" :loading="isLoading" :searchable="true" :empty-text="$t('admin.ai.noTools')" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '@/features/ai/tools/list/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useEmitter } from '@/composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const tools = ref([])
const { t } = useI18n()
const isLoading = ref(false)
const emit = useEmitter()
const columns = computed(() => createColumns(t, { onDelete: deleteTool }))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'ai_tool') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const params = { page: 1, page_size: 500 }
    const resp = await api.getAITools(params)
    tools.value = resp.data.data?.results ?? []
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

const deleteTool = async (tool) => {
  try {
    await api.deleteAITool(tool.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'ai_tool' })
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.deletedSuccessfully')
    })
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}
</script>
