<template>
  <div class="flex flex-col h-full">
    <!-- Top bar: back + inline title input + Save. Title in edit mode does
         NOT regenerate the slug — slug is set on creation only. -->
    <div class="flex items-center gap-2 px-4 py-2 border-b border-border">
      <Button variant="ghost" size="icon" class="shrink-0" @click="onCancel" :title="t('globals.messages.cancel')">
        <ArrowLeft class="size-4" />
      </Button>
      <input
        v-model="kb.fields.title"
        type="text"
        :placeholder="t('globals.terms.title')"
        class="flex-1 min-w-0 h-9 bg-transparent border-0 outline-none px-2 text-base font-medium rounded hover:bg-muted/50 focus:bg-muted/50 focus:ring-1 focus:ring-ring transition-colors placeholder:text-muted-foreground/60"
      />
      <Button variant="ghost" type="button" class="shrink-0" @click="onCancel">
        {{ t('globals.messages.cancel') }}
      </Button>
      <Button type="button" class="shrink-0" @click="onSave">
        {{ t('globals.messages.save') }}
      </Button>
    </div>
    <!-- Title error strip -->
    <p
      v-if="kb.errors.title"
      class="text-xs text-destructive px-6 py-1.5 border-b border-border bg-destructive/5"
    >
      {{ kb.errors.title }}
    </p>

    <!-- Body: editor full-width, full-height. -->
    <div class="flex-1 overflow-hidden">
      <ArticleForm v-if="initialBody" ref="formRef" :initial-body="initialBody" />
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeft } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button/index.js'
import ArticleForm from '@/features/knowledge/ArticleForm.vue'
import { useKnowledgeStore } from '@main/stores/knowledge'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api/index.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const emitter = useEmitter()
const kb = useKnowledgeStore()
const formRef = ref(null)

const initialBody = ref(null)

onMounted(async () => {
  const id = Number(route.params.id)
  const [a] = await Promise.all([
    api.getKnowledgeArticle(id),
    kb.folders.length === 0 ? kb.fetchFolders() : Promise.resolve()
  ])
  const article = a.data.data
  kb.enterEditMode(article)
  initialBody.value = {
    html: article.body_html || '',
    json: article.body_tiptap_json || { type: 'doc', content: [] },
    markdown: article.body_markdown || ''
  }
})

onBeforeUnmount(() => kb.exitFormMode())

const onSave = async () => {
  if (!kb.validate(t)) return
  try {
    const body = formRef.value?.readBody?.() || initialBody.value
    const payload = {
      title: kb.fields.title,
      slug: kb.fields.slug, // unchanged from server-loaded value
      locale: kb.fields.locale,
      classification: kb.fields.classification,
      is_ai_groundable: kb.fields.is_ai_groundable,
      body_html: body.html,
      body_tiptap_json: body.json,
      body_markdown: body.markdown
    }
    if (kb.fields.folder_id !== kb.initialFolderId) {
      await api.moveKnowledgeArticle(kb.articleId, { folder_id: kb.fields.folder_id })
    }
    await api.updateKnowledgeArticle(kb.articleId, payload)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

const onCancel = () => router.push({ name: 'knowledge-list' })
</script>
