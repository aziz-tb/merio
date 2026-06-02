<template>
  <PageHeader />
  <AgentForm :submitForm="onSubmit" :initialValues="{}" :isNewForm="true" :isLoading="formLoading" />
</template>

<script setup>
import { ref } from 'vue'
import AgentForm from '@/features/admin/agents/AgentForm.vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useRouter } from 'vue-router'
import { useEmitter } from '../../../composables/useEmitter'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { useI18n } from 'vue-i18n'
import api from '../../../api'

const { t } = useI18n()
const emitter = useEmitter()
const router = useRouter()
const formLoading = ref(false)

const onSubmit = (values) => {
  createNewUser(values)
}

const createNewUser = async (values) => {
  try {
    formLoading.value = true
    await api.createUser(values)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
    router.push({ name: 'agent-list' })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    formLoading.value = false
  }
}
</script>
