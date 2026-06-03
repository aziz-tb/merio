<template>
  <PageHeader :description="description">
    <template #actions>
      <Dialog v-model:open="dialogOpen">
        <DialogTrigger as-child @click="newStatus">
          <Button>
            {{
              $t('status.new')
            }}
          </Button>
        </DialogTrigger>
        <DialogContent class="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>
              {{
                isEditing
                  ? $t('status.edit')
                  : $t('status.new')
              }}
            </DialogTitle>
            <DialogDescription>
              {{ $t('admin.conversationStatus.name.description') }}
            </DialogDescription>
          </DialogHeader>
          <StatusForm @submit.prevent="onSubmit">
            <template #footer>
              <DialogFooter class="mt-10">
                <Button type="submit" :isLoading="isLoading" :disabled="isLoading">
                  {{ isEditing ? $t('globals.messages.save') : $t('globals.messages.create') }}
                </Button>
              </DialogFooter>
            </template>
          </StatusForm>
        </DialogContent>
      </Dialog>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="statuses" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/status/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import StatusForm from '@/features/admin/status/StatusForm.vue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@shared-ui/components/ui/dialog'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { createFormSchema } from '../../../features/admin/status/formSchema.js'
import { useEmitter } from '../../../composables/useEmitter'
import { useNavDescription } from '@main/composables/useNavDescription'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useI18n } from 'vue-i18n'
import { FALLBACK_STATUS_VISUAL } from '@main/utils/statusVisuals'
import api from '../../../api'

const { t } = useI18n()
const isLoading = ref(false)
const statuses = ref([])
const emit = useEmitter()
const dialogOpen = ref(false)
const isEditing = ref(false)
const editingId = ref(null)
const description = useNavDescription()
const columns = computed(() => createColumns(t, {
  onEdit: editStatus,
  onDelete: deleteStatus
}))

const refreshHandler = (data) => {
  if (data?.model === 'status') getStatuses()
}
const editHandler = (data) => {
  if (data?.model === 'status') {
    editStatus(data.data)
  }
}

onMounted(() => {
  getStatuses()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emit.on(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emit.off(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

const form = useForm({
  validationSchema: toTypedSchema(createFormSchema(t)),
  initialValues: {
    color: FALLBACK_STATUS_VISUAL.color,
    icon: FALLBACK_STATUS_VISUAL.icon
  }
})

const editStatus = (item) => {
  editingId.value = item.id
  form.setValues(item)
  form.setErrors({})
  isEditing.value = true
  dialogOpen.value = true
}

const newStatus = () => {
  form.resetForm({
    values: {
      color: FALLBACK_STATUS_VISUAL.color,
      icon: FALLBACK_STATUS_VISUAL.icon
    }
  })
  form.setErrors({})
  isEditing.value = false
}

const getStatuses = async () => {
  try {
    isLoading.value = true
    const resp = await api.getStatuses()
    statuses.value = resp.data.data
  } finally {
    isLoading.value = false
  }
}

const deleteStatus = async (status) => {
  try {
    await api.deleteStatus(status.id)
    emit.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'status' })
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

const onSubmit = form.handleSubmit(async (values) => {
  try {
    isLoading.value = true
    if (isEditing.value) {
      await api.updateStatus(editingId.value, values)
    } else {
      await api.createStatus(values)
    }
    dialogOpen.value = false
    getStatuses()
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
})
</script>
