<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-team' }">
        <Button> {{ $t('globals.messages.new') }} </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="data" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { createColumns } from '../../../features/admin/teams/TeamsDataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useEmitter } from '../../../composables/useEmitter'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import DataTable from '@main/components/datatable/DataTable.vue'
import { useI18n } from 'vue-i18n'
import api from '../../../api'

const emitter = useEmitter()
const { t } = useI18n()
const data = ref([])
const isLoading = ref(false)
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteTeam }))

const getData = async () => {
  try {
    isLoading.value = true
    const response = await api.getTeams()
    data.value = response.data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      title: 'Error',
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

const deleteTeam = async (team) => {
  try {
    await api.deleteTeam(team.id)
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, {
      model: 'team'
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      title: 'Error',
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

const refreshHandler = (event) => {
  if (event.model === 'team') {
    getData()
  }
}

const listenForRefresh = () => {
  emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
}

const removeListeners = () => {
  emitter.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
}

onMounted(async () => {
  getData()
  listenForRefresh()
})

onUnmounted(() => {
  removeListeners()
})
</script>
