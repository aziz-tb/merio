<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-business-hours' }">
        <Button>
          {{
            $t('businessHour.new')
          }}
        </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="businessHours" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { Button } from '@shared-ui/components/ui/button'
import { useEmitter } from '../../../composables/useEmitter'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { useI18n } from 'vue-i18n'
import { createColumns } from '../../../features/admin/business-hours/dataTableColumns.js'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import api from '../../../api'

const { t } = useI18n()
const businessHours = ref([])
const isLoading = ref(false)
const emit = useEmitter()
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteBusinessHours }))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'business_hours') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const resp = await api.getAllBusinessHours()
    businessHours.value = resp.data.data
  } finally {
    isLoading.value = false
  }
}

const deleteBusinessHours = async (businessHour) => {
  await api.deleteBusinessHours(businessHour.id)
  emit.emit(EMITTER_EVENTS.REFRESH_LIST, {
    model: 'business_hours'
  })
}
</script>
