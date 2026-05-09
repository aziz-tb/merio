// Pinia store backing the knowledge-base UI.
//
// Two surfaces share state through this store:
//   1. KnowledgeList.vue (article table) + Sidebar.vue's knowledge-list mode
//      (folder tree, "All articles" selector, "+ New folder" button).
//   2. CreateArticle.vue / EditArticle.vue (full-page editor) + Sidebar.vue's
//      knowledge-form mode (status badge, folder picker, slug, locale,
//      classification, AI-grounded toggle).
//
// The article body (HTML / TipTap JSON / Markdown) is NOT stored here —
// it lives in the KbArticleEditor instance and is read fresh on save via
// `bodyConvert.getBody(editor)`. Only metadata fields are stored.
import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import api from '@/api/index.js'

const emptyFields = () => ({
  title: '',
  slug: '',
  folder_id: null,
  locale: 'en',
  classification: 'faq',
  is_ai_groundable: false
})

export const useKnowledgeStore = defineStore('knowledge', () => {
  // -------------------------------------------------------------------------
  // List-mode state
  // -------------------------------------------------------------------------
  const folders = ref([])
  const selectedFolderId = ref(null)

  const folderById = computed(() => {
    const m = {}
    for (const f of folders.value) m[f.id] = f
    return m
  })

  const fetchFolders = async () => {
    const resp = await api.getKnowledgeFolders()
    folders.value = resp.data.data || []
  }

  // -------------------------------------------------------------------------
  // Form-mode state — populated by Create/EditArticle on mount, cleared on unmount.
  // -------------------------------------------------------------------------
  const formMode = ref(null) // 'create' | 'edit' | null
  const articleId = ref(null)
  const articleStatus = ref('draft')
  const fields = reactive(emptyFields())
  // Snapshot of folder_id when entering edit mode — used to detect a folder
  // change at save time (folder change requires the separate Move endpoint).
  const initialFolderId = ref(null)
  // Field-level validation errors, populated by validate() on save.
  const errors = reactive({})

  const isFormMode = computed(() => formMode.value !== null)

  const enterCreateMode = () => {
    formMode.value = 'create'
    articleId.value = null
    articleStatus.value = 'draft'
    Object.assign(fields, emptyFields())
    initialFolderId.value = null
    clearErrors()
  }

  const enterEditMode = (article) => {
    formMode.value = 'edit'
    articleId.value = article.id
    articleStatus.value = article.status
    fields.title = article.title
    fields.slug = article.slug
    fields.folder_id = article.folder_id
    fields.locale = article.locale
    fields.classification = article.classification
    fields.is_ai_groundable = article.is_ai_groundable
    initialFolderId.value = article.folder_id
    clearErrors()
  }

  const exitFormMode = () => {
    formMode.value = null
    articleId.value = null
    Object.assign(fields, emptyFields())
    initialFolderId.value = null
    clearErrors()
  }

  const setStatus = (status) => {
    articleStatus.value = status
  }

  const clearErrors = () => {
    for (const k of Object.keys(errors)) delete errors[k]
  }

  // Lightweight inline validation — replaces vee-validate now that the form
  // fields straddle two components (page + sidebar) and a shared form
  // instance isn't practical.
  const SLUG_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/
  const VALID_LOCALES = new Set(['en', 'de', 'fr'])
  const VALID_CLASS = new Set(['faq', 'how_to', 'policy', 'news', 'other'])

  // Slug is auto-derived (never user-edited), so its only failure path here
  // is "couldn't derive a non-empty slug from the title" — surface that as a
  // title error rather than a slug-specific message the user can't act on.
  const validate = (t) => {
    clearErrors()
    if (!fields.title || !fields.title.trim()) {
      errors.title = t('globals.messages.required')
    } else if (fields.title.length > 500) {
      errors.title = t('admin.knowledge.title.tooLong')
    } else if (!fields.slug || !SLUG_RE.test(fields.slug)) {
      // Title contained only characters slugify can't reduce (e.g. punctuation
      // or non-Latin scripts that strip to empty). Ask the user to adjust.
      errors.title = t('admin.knowledge.slug.invalid')
    }
    if (!fields.folder_id) {
      errors.folder_id = t('admin.knowledge.folder.required')
    }
    if (!VALID_LOCALES.has(fields.locale)) {
      errors.locale = t('admin.knowledge.locale.invalid')
    }
    if (!VALID_CLASS.has(fields.classification)) {
      errors.classification = t('admin.knowledge.classification.invalid')
    }
    return Object.keys(errors).length === 0
  }

  return {
    // List mode
    folders,
    selectedFolderId,
    folderById,
    fetchFolders,
    // Form mode
    formMode,
    isFormMode,
    articleId,
    articleStatus,
    fields,
    initialFolderId,
    errors,
    enterCreateMode,
    enterEditMode,
    exitFormMode,
    setStatus,
    validate,
    clearErrors
  }
})
