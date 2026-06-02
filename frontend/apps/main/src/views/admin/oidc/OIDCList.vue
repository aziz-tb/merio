<template>
  <PageHeader :description="description">
    <template #actions>
      <RouterLink :to="{ name: 'new-sso' }">
        <Button>{{
          $t('oidc.new')
        }}</Button>
      </RouterLink>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="oidc" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/oidc/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import { useEmitter } from '../../../composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import api from '../../../api'

const oidc = ref([])
const { t } = useI18n()
const isLoading = ref(false)
const emit = useEmitter()
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteOIDC }))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'oidc') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const resp = await api.getAllOIDC()
    oidc.value = resp.data.data
  } finally {
    isLoading.value = false
  }
}

const deleteOIDC = async (oidcProvider) => {
  await api.deleteOIDC(oidcProvider.id)
  emit.emit(EMITTER_EVENTS.REFRESH_LIST, {
    model: 'oidc'
  })
}
</script>
