<template>
  <PageHeader :description="description" />
  <div :class="{ 'opacity-50 transition-opacity duration-300': isLoading }">
    <Spinner v-if="isLoading" />
    <NotificationsForm :initial-values="initialValues" :submit-form="submitForm" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '@main/api'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { useI18n } from 'vue-i18n'
import NotificationsForm from './NotificationSettingForm.vue'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { useAppSettingsStore } from '@main/stores/appSettings'

const initialValues = ref({})
const { t } = useI18n()
const isLoading = ref(false)
const emitter = useEmitter()
const appSettingsStore = useAppSettingsStore()
const description = useNavDescription()

onMounted(() => {
  getNotificationSettings()
})

const getNotificationSettings = async () => {
  try {
    isLoading.value = true
    const resp = await api.getEmailNotificationSettings()
    initialValues.value = Object.fromEntries(
      Object.entries(resp.data.data).map(([key, value]) => [
        key.replace('notification.email.', ''),
        value
      ])
    )
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

const submitForm = async (values) => {
  try {
    const updatedValues = Object.fromEntries(
      Object.entries(values).map(([key, value]) => {
        if (key === 'password' && value.includes('•')) {
          return [`notification.email.${key}`, '']
        }
        return [`notification.email.${key}`, value]
      })
    )
    await api.updateEmailNotificationSettings(updatedValues)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.notification.restartApp')
    })
    await getNotificationSettings()
    appSettingsStore.fetchSettings()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}
</script>
