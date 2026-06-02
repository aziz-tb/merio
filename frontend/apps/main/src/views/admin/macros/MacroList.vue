<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-macro' }">
        <Button>
          {{
            $t('macro.new')
          }}
        </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="formLoading" reserve-height>
    <DataTable :columns="columns" :data="macros" :loading="formLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/macros/dataTableColumns.js'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useEmitter } from '../../../composables/useEmitter'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import { useI18n } from 'vue-i18n'
import { useMacroStore } from '@main/stores/macro'
import api from '../../../api'

const { t } = useI18n()
const formLoading = ref(false)
const macros = ref([])
const emit = useEmitter()
const macroStore = useMacroStore()
const description = useNavDescription()
const columns = computed(() => createColumns(t, { onDelete: deleteMacro }))

onMounted(() => {
  getMacros()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'macros') getMacros()
}

const getMacros = async () => {
  try {
    formLoading.value = true
    const resp = await api.getAllMacros()
    macros.value = resp.data.data
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    formLoading.value = false
  }
}

const deleteMacro = async (macro) => {
  await api.deleteMacro(macro.id)
  await macroStore.loadMacros(true)
  emit.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'macros' })
}
</script>
