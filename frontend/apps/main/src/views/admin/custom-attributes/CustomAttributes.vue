<template>
  <Tabs default-value="contact" v-model="appliesTo">
    <PageHeader :description="description">
      <template #actions>
        <Dialog v-model:open="dialogOpen">
          <DialogTrigger as-child @click="newCustomAttribute">
            <Button>
              {{
                $t('customAttribute.new')
              }}
            </Button>
          </DialogTrigger>
          <DialogContent class="sm:max-w-[600px]">
            <DialogHeader>
              <DialogTitle>
                {{
                  isEditing
                    ? $t('customAttribute.edit')
                    : $t('customAttribute.new')
                }}
              </DialogTitle>
              <DialogDescription/>
            </DialogHeader>
            <CustomAttributesForm @submit.prevent="onSubmit" :form="form">
              <template #footer>
                <DialogFooter class="mt-10">
                  <Button type="submit" :isLoading="isLoading">
                    {{
                      isEditing ? $t('globals.messages.save') : $t('globals.messages.create')
                    }}
                  </Button>
                </DialogFooter>
              </template>
            </CustomAttributesForm>
          </DialogContent>
        </Dialog>
      </template>
      <template #tabs>
        <TabsList class="grid w-full grid-cols-2">
          <TabsTrigger value="contact">
            {{ $t('globals.terms.contact') }}
          </TabsTrigger>
          <TabsTrigger value="conversation">
            {{ $t('globals.terms.conversation') }}
          </TabsTrigger>
        </TabsList>
      </template>
    </PageHeader>
    <LoadingOverlay :loading="isLoading" reserve-height>
      <TabsContent value="contact">
        <DataTable :columns="columns" :data="customAttributes" :loading="isLoading" :searchable="false" />
      </TabsContent>
      <TabsContent value="conversation">
        <DataTable :columns="columns" :data="customAttributes" :loading="isLoading" :searchable="false" />
      </TabsContent>
    </LoadingOverlay>
  </Tabs>
</template>

<script setup>
import { computed, ref, onMounted, watch, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '../../../features/admin/custom-attributes/dataTableColumns.js'
import CustomAttributesForm from '@/features/admin/custom-attributes/CustomAttributesForm.vue'
import { Button } from '@shared-ui/components/ui/button'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { createFormSchema } from '../../../features/admin/custom-attributes/formSchema.js'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { useEmitter } from '../../../composables/useEmitter'
import { EMITTER_EVENTS } from '../../../constants/emitterEvents.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@shared-ui/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared-ui/components/ui/tabs'
import { useStorage } from '@vueuse/core'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { useI18n } from 'vue-i18n'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '../../../api'

const appliesTo = useStorage('appliesTo', 'contact')
const { t } = useI18n()
const customAttributes = ref([])
const isLoading = ref(false)
const emitter = useEmitter()
const dialogOpen = ref(false)
const isEditing = ref(false)
const description = useNavDescription()
const columns = computed(() => createColumns(t, {
  onEdit: editCustomAttribute,
  onDelete: deleteCustomAttribute
}))

const refreshHandler = (data) => {
  if (data?.model === 'custom-attributes') fetchAll()
}
const editHandler = (data) => {
  if (data?.model === 'custom-attributes') {
    form.setValues(data.data)
    form.setErrors({})
    isEditing.value = true
    dialogOpen.value = true
  }
}

onMounted(async () => {
  fetchAll()
  emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emitter.on(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emitter.off(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

const editCustomAttribute = (item) => {
  form.setValues(item)
  form.setErrors({})
  isEditing.value = true
  dialogOpen.value = true
}

const newCustomAttribute = () => {
  form.resetForm()
  form.setErrors({})
  isEditing.value = false
  dialogOpen.value = true
}

const deleteCustomAttribute = async (customAttribute) => {
  try {
    await api.deleteCustomAttribute(customAttribute.id)
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, {
      model: 'custom-attributes'
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  }
}

const form = useForm({
  validationSchema: toTypedSchema(createFormSchema(t)),
  initialValues: {
    id: 0,
    name: '',
    data_type: 'text',
    applies_to: appliesTo.value,
    values: []
  }
})

const fetchAll = async () => {
  if (!appliesTo.value) return
  try {
    isLoading.value = true
    const resp = await api.getCustomAttributes(appliesTo.value)
    customAttributes.value = resp.data.data
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

const onSubmit = form.handleSubmit(async (values) => {
  try {
    isLoading.value = true
    if (values.id) {
      await api.updateCustomAttribute(values.id, values)
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        description: t('globals.messages.savedSuccessfully')
      })
    } else {
      await api.createCustomAttribute(values)
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        description: t('globals.messages.savedSuccessfully')
      })
    }

    dialogOpen.value = false
    fetchAll()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
})

watch(
  appliesTo,
  (newVal) => {
    form.resetForm({
      values: {
        ...form.values,
        applies_to: newVal
      }
    })
    fetchAll()
  },
  { immediate: true }
)
</script>
