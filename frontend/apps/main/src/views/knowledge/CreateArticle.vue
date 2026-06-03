<template>
  <div class="flex flex-col h-full">
    <!-- Top bar: back + inline title input + Save. -->
    <div class="flex items-center gap-2 px-4 py-2 border-b border-border">
      <Button variant="ghost" size="icon" class="shrink-0" @click="onCancel" :title="t('globals.messages.cancel')">
        <ArrowLeft class="size-4" />
      </Button>
      <input
        v-model="kb.fields.title"
        type="text"
        :placeholder="t('globals.terms.title')"
        class="flex-1 min-w-0 h-9 bg-transparent border-0 outline-none px-2 text-base font-medium rounded hover:bg-muted/50 focus:bg-muted/50 focus:ring-1 focus:ring-ring transition-colors placeholder:text-muted-foreground/60"
        @input="onTitleInput"
      />
      <Button variant="ghost" type="button" class="shrink-0" @click="onCancel">
        {{ t('globals.messages.cancel') }}
      </Button>
      <Button type="button" class="shrink-0" @click="onSave">
        {{ t('globals.messages.create') }}
      </Button>
    </div>
    <!-- Title error strip (only shown when validation fails) -->
    <p
      v-if="kb.errors.title"
      class="text-xs text-destructive px-6 py-1.5 border-b border-border bg-destructive/5"
    >
      {{ kb.errors.title }}
    </p>

    <!-- Body: editor full-width, full-height. -->
    <div class="flex-1 overflow-hidden">
      <ArticleForm ref="formRef" />
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeft } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button/index.js'
import ArticleForm from '@/features/knowledge/ArticleForm.vue'
import { useKnowledgeStore } from '@main/stores/knowledge'
import { useEmitter } from '@/composables/useEmitter.js'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { slugify } from '@/utils/slugify.js'
import api from '@/api/index.js'

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const kb = useKnowledgeStore()
const formRef = ref(null)

onMounted(async () => {
  kb.enterCreateMode()
  if (kb.folders.length === 0) await kb.fetchFolders()
})

onBeforeUnmount(() => kb.exitFormMode())

// In create mode the slug auto-tracks the title. (Edit mode never regenerates
// slug; that lives in EditArticle.vue, which omits this handler.)
const onTitleInput = (e) => {
  kb.fields.slug = slugify(e.target.value)
}

const onSave = async () => {
  // Defensive: ensure slug is set even if the @input handler missed (paste
  // event, IME, etc.). Server validates the final value.
  if (!kb.fields.slug && kb.fields.title) {
    kb.fields.slug = slugify(kb.fields.title)
  }
  if (!kb.validate(t)) return // inline errors visible

  try {
    const body = formRef.value?.readBody?.() || {
      html: '',
      json: { type: 'doc', content: [] },
      markdown: ''
    }
    const payload = {
      folder_id: kb.fields.folder_id,
      title: kb.fields.title,
      slug: kb.fields.slug,
      locale: kb.fields.locale,
      classification: kb.fields.classification,
      is_ai_groundable: kb.fields.is_ai_groundable,
      body_html: body.html,
      body_tiptap_json: body.json,
      body_markdown: body.markdown
    }
    const resp = await api.createKnowledgeArticle(payload)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('globals.messages.savedSuccessfully')
    })
    router.push({ name: 'edit-article', params: { id: resp.data.data.id } })
  } catch (e) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(e).message
    })
  }
}

const onCancel = () => router.push({ name: 'knowledge-list' })
</script>
