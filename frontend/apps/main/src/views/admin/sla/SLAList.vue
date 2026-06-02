<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-sla' }">
        <Button>
          {{
            t('sla.new')
          }}
        </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="slas" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/sla/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import { useEmitter } from '../../../composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '../../../api'

const { t } = useI18n()
const slas = ref([])
const isLoading = ref(false)
const emit = useEmitter()
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteSLA }))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'sla') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const resp = await api.getAllSLAs()
    slas.value = resp.data.data
  } finally {
    isLoading.value = false
  }
}

const deleteSLA = async (sla) => {
  try {
    await api.deleteSLA(sla.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, {
      model: 'sla'
    })
  } catch (err) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(err).message
    })
  }
}
</script>
