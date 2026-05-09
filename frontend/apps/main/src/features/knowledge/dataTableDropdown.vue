<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" class="w-8 h-8 p-0">
        <span class="sr-only">{{ t('globals.terms.actions') }}</span>
        <MoreHorizontal class="w-4 h-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent>
      <DropdownMenuItem @click="editArticle">
        {{ t('globals.messages.edit') }}
      </DropdownMenuItem>
      <DropdownMenuItem v-if="article.status === 'draft'" @click="publishArticle">
        {{ t('admin.knowledge.actions.publish') }}
      </DropdownMenuItem>
      <DropdownMenuItem v-else @click="unpublishArticle">
        {{ t('admin.knowledge.actions.unpublish') }}
      </DropdownMenuItem>
      <DropdownMenuItem @click="() => (alertOpen = true)">
        {{ t('globals.messages.delete') }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>

  <AlertDialog :open="alertOpen" @update:open="alertOpen = $event">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ t('admin.knowledge.deleteConfirmation') }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ t('globals.messages.cancel') }}</AlertDialogCancel>
        <AlertDialogAction @click="deleteArticle">{{ t('globals.messages.delete') }}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { MoreHorizontal } from 'lucide-vue-next'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu/index.js'
import { Button } from '@shared-ui/components/ui/button/index.js'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@shared-ui/components/ui/alert-dialog/index.js'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { useI18n } from 'vue-i18n'
import api from '@/api/index.js'

const { t } = useI18n()
const router = useRouter()
const alertOpen = ref(false)
const emitter = useEmitter()

const props = defineProps({
  article: { type: Object, required: true }
})

const editArticle = () => {
  router.push({ name: 'edit-article', params: { id: props.article.id } })
}

const publishArticle = async () => {
  try {
    await api.publishKnowledgeArticle(props.article.id)
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'knowledge-articles' })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: e?.response?.data?.message || e.message
    })
  }
}

const unpublishArticle = async () => {
  try {
    await api.unpublishKnowledgeArticle(props.article.id)
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'knowledge-articles' })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: e?.response?.data?.message || e.message
    })
  }
}

const deleteArticle = async () => {
  try {
    await api.deleteKnowledgeArticle(props.article.id)
  } finally {
    alertOpen.value = false
    emitter.emit(EMITTER_EVENTS.REFRESH_LIST, { model: 'knowledge-articles' })
  }
}
</script>
