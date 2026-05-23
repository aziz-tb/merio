<template>
  <AdminSplitLayout>
    <template #content>
      <LoadingOverlay :loading="isLoading" reserve-height>
        <div class="mb-5 flex justify-end gap-2">
          <Importer
            entity-key="globals.terms.tag"
            :upload-fn="api.importTags"
            :get-status-fn="api.getTagImportStatus"
            @import-complete="handleImportComplete"
          >
            <template #csv-example>
              <div class="bg-muted p-3 rounded text-xs font-mono overflow-x-auto leading-relaxed">
                <div>name</div>
                <div>Bug</div>
                <div>Feature Request</div>
                <div>Billing</div>
              </div>
            </template>
          </Importer>
          <Button @click="newTag">{{ t('tag.new') }}</Button>
        </div>

        <TagsList :tags="tags" @edit="editTag" />
      </LoadingOverlay>
    </template>

    <template #help>
      <p>{{ $t('admin.tag.help') }}</p>
    </template>
  </AdminSplitLayout>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import AdminSplitLayout from '@/layouts/admin/AdminSplitLayout.vue'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { Button } from '@shared-ui/components/ui/button'
import TagsList from '@/features/admin/tags/TagsList.vue'
import Importer from '@/components/importer/Importer.vue'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { useTagStore } from '@/stores/tag'
import { useI18n } from 'vue-i18n'
import api from '@/api'

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const tagStore = useTagStore()
const isLoading = ref(false)
const tags = ref([])

const refreshHandler = async (data) => {
  if (data?.model === 'tags') {
    await tagStore.fetchTags(true)
    await getTags()
  }
}
const editHandler = (data) => {
  if (data?.model === 'tags') editTag(data.data)
}

onMounted(async () => {
  await getTags()
  emitter.on(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emitter.on(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

onUnmounted(() => {
  emitter.off(EMITTER_EVENTS.REFRESH_LIST, refreshHandler)
  emitter.off(EMITTER_EVENTS.EDIT_MODEL, editHandler)
})

const getTags = async () => {
  isLoading.value = true
  try {
    const resp = await api.getTags()
    tags.value = resp.data.data || []
  } finally {
    isLoading.value = false
  }
}

const handleImportComplete = async () => {
  await tagStore.fetchTags(true)
  await getTags()
}

const newTag = () => {
  router.push({ name: 'new-tag' })
}

const editTag = (tag) => {
  router.push({ name: 'edit-tag', params: { id: tag.id } })
}
</script>
