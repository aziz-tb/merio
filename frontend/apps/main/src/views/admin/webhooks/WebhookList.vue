<template>
  <PageHeader :description="description">
    <template #actions>
      <RouterLink :to="{ name: 'new-webhook' }">
        <Button>{{
          $t('webhook.new')
        }}</Button>
      </RouterLink>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="webhooks" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/webhooks/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import { useEmitter } from '../../../composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '../../../api'

const webhooks = ref([])
const { t } = useI18n()
const isLoading = ref(false)
const emit = useEmitter()
const description = useNavDescription()
const columns = computed(() => createColumns(t, {
  onDelete: deleteWebhook,
  onToggle: toggleWebhook,
  onTest: testWebhook
}))

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'webhook') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const resp = await api.getWebhooks()
    webhooks.value = resp.data.data
  } finally {
    isLoading.value = false
  }
}

const deleteWebhook = async (webhook) => {
  try {
    await api.deleteWebhook(webhook.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, {
      model: 'webhook'
    })
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

const toggleWebhook = async (webhook) => {
  try {
    await api.toggleWebhook(webhook.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, {
      model: 'webhook'
    })
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'success',
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

const testWebhook = async (webhook) => {
  try {
    await api.testWebhook(webhook.id)
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'success',
      description: t('webhook.sentSuccessfully')
    })
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}
</script>
