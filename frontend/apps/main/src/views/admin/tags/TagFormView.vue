<template>
  <AdminSplitLayout>
    <template #content>
      <div class="mb-5">
        <CustomBreadcrumb :links="breadcrumbLinks" />
      </div>

      <LoadingOverlay :loading="isLoading" reserve-height>
        <div class="space-y-5">
          <TagsForm
            v-model="formData"
            :tags="tags"
            :slas="slas"
            :priorities="priorities"
            :is-editing="isEditing"
            @submit="save"
          />

          <div class="flex justify-end gap-2 border-t border-border pt-4">
            <Button type="button" variant="ghost" @click="goBack">
              {{ t('globals.messages.cancel') }}
            </Button>
            <Button type="button" :isLoading="saving" @click="save">
              {{ isEditing ? t('globals.messages.save') : t('globals.messages.create') }}
            </Button>
          </div>
        </div>
      </LoadingOverlay>
    </template>

    <template #help>
      <p>{{ $t('admin.tag.help') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { CustomBreadcrumb } from '@shared-ui/components/ui/breadcrumb'
import { Button } from '@shared-ui/components/ui/button'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import AdminSplitLayout from '@/layouts/admin/AdminSplitLayout.vue'
import TagsForm from '@/features/admin/tags/TagsForm.vue'
import { useTagStore } from '@/stores/tag'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const props = defineProps({
  id: {
    type: [String, Number],
    required: false
  }
})

const router = useRouter()
const { t } = useI18n()
const emitter = useEmitter()
const tagStore = useTagStore()

const isLoading = ref(false)
const saving = ref(false)
const formData = ref(defaultTag())
const tags = ref([])
const slas = ref([])
const priorities = ref([])

const isEditing = computed(() => Boolean(props.id))
const breadcrumbLinks = computed(() => [
  { path: 'tags-list', label: t('globals.terms.tag', 2) },
  { path: '', label: isEditing.value ? t('tag.edit') : t('tag.new') }
])

function defaultTag () {
  return {
    name: '',
    parent_id: null,
    type: 'skill',
    color: 'grey',
    visibility: 'public',
    description: '',
    detection: { mode: 'manual_only', rules: { op: 'AND', children: [] }, ai: { prompt: '' } },
    sla_policy_id: null,
    priority_id: null
  }
}

const cloneTag = (tag) => ({
  ...defaultTag(),
  ...JSON.parse(JSON.stringify(tag || {})),
  detection: tag?.detection || defaultTag().detection
})

const normalizeID = (value) => {
  if (value && typeof value === 'object') return value.Valid ? value.Int : null
  if (value === '' || value === undefined) return null
  return value
}

const payloadFromForm = () => ({
  name: formData.value.name,
  parent_id: normalizeID(formData.value.parent_id),
  type: formData.value.type,
  color: formData.value.color,
  visibility: formData.value.visibility,
  description: formData.value.description || '',
  detection: formData.value.detection || defaultTag().detection,
  sla_policy_id: normalizeID(formData.value.sla_policy_id),
  priority_id: normalizeID(formData.value.priority_id)
})

const loadForm = async () => {
  isLoading.value = true
  try {
    const [tagsResp, slasResp, prioritiesResp] = await Promise.all([
      api.getTags(),
      api.getAllSLAs(),
      api.getPriorities()
    ])
    tags.value = tagsResp.data.data || []
    slas.value = slasResp.data.data || []
    priorities.value = prioritiesResp.data.data || []

    if (isEditing.value) {
      const tagResp = await api.getTag(props.id)
      formData.value = cloneTag(tagResp.data.data)
    } else {
      formData.value = defaultTag()
    }
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message || t('tag.notFound')
    })
    router.replace({ name: 'tags-list' })
  } finally {
    isLoading.value = false
  }
}

const save = async () => {
  if (!formData.value.name || formData.value.name.trim().length < 3) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('admin.conversationTags.name.valid')
    })
    return
  }

  saving.value = true
  try {
    const payload = payloadFromForm()
    if (isEditing.value) {
      await api.updateTag(formData.value.id, payload)
    } else {
      await api.createTag(payload)
    }
    await tagStore.fetchTags(true)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
    router.push({ name: 'tags-list' })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    saving.value = false
  }
}

const goBack = () => {
  router.push({ name: 'tags-list' })
}

onMounted(loadForm)
</script>
