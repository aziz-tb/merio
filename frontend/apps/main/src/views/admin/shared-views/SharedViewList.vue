<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-shared-view' }">
        <Button>
          {{
            $t('sharedView.new')
          }}
        </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="formLoading" reserve-height>
    <DataTable :columns="columns" :data="sharedViews" :loading="formLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@/components/datatable/DataTable.vue'
import { createColumns } from '@/features/admin/shared-views/dataTableColumns.js'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import { useEmitter } from '@/composables/useEmitter'
import { useNavDescription } from '@/composables/useNavDescription'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { useI18n } from 'vue-i18n'
import { useSharedViewStore } from '@/stores/sharedView'
import api from '@/api'

const { t } = useI18n()
const formLoading = ref(false)
const sharedViews = ref([])
const emit = useEmitter()
const sharedViewStore = useSharedViewStore()
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteSharedView }))

onMounted(() => {
  getSharedViews()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'shared-views') getSharedViews()
}

const getSharedViews = async () => {
  try {
    formLoading.value = true
    const resp = await api.getAllSharedViews()
    sharedViews.value = resp.data.data
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    formLoading.value = false
  }
}

const deleteSharedView = async (sharedView) => {
  await api.deleteSharedView(sharedView.id)
  await sharedViewStore.refresh()
  emit.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'shared-views' })
}
</script>
