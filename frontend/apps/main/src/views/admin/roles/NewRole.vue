<template>
  <PageHeader />
  <RoleForm :initial-values="{}" :submitForm="submitForm" :isLoading="formLoading" :isNewForm="true" />
</template>

<script setup>
import { ref } from 'vue'
import RoleForm from '@/features/admin/roles/RoleForm.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import api from '../../../api'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { useEmitter } from '../../../composables/useEmitter'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

const emitter = useEmitter()
const { t } = useI18n()
const router = useRouter()
const formLoading = ref(false)

const submitForm = async (values) => {
  try {
    formLoading.value = true
    await api.createRole(values)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
    router.push({ name: 'role-list' })
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
