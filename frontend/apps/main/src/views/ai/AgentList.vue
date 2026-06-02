<template>
  <PageHeader :description="t('admin.ai.agent.help')">
    <template #actions>
      <RouterLink :to="{ name: 'agent-create-identity' }">
        <Button>{{ $t('aiAgent.new') }}</Button>
      </RouterLink>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="agents" :loading="isLoading" :searchable="true" :empty-text="$t('admin.ai.noAgents')" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '@/features/ai/agents/list/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useEmitter } from '@/composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const agents = ref([])
const { t } = useI18n()
const isLoading = ref(false)
const emit = useEmitter()
const columns = computed(() => createColumns(t, { onDelete: deleteAgent }))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'ai_agent') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const params = { page: 1, page_size: 500 }
    const resp = await api.getAIAgents(params)
    agents.value = resp.data.data?.results ?? []
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

const deleteAgent = async (agent) => {
  try {
    await api.deleteAIAgent(agent.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'ai_agent' })
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
