// useAgentEdit — agent record + dirty tracking + Save action for the
// three-rail Agent editor layout.
//
// Owns:
//   - `agent` ref: the live, mutable record. Tabs mutate it directly.
//   - `original` ref: deep-clone snapshot of the last-saved state.
//   - `isDirty` computed: deep-equal compare of agent vs original.
//   - `saving` ref + `save()` action: POST for drafts, PUT for existing
//     agents, then refresh the snapshot.
//   - Browser-close guard via `beforeunload`. Registered in onMounted,
//     unregistered in onUnmounted so it doesn't leak across navigations.
//
// The composable is constructed once by the route-scoped layout
// provider above both the sidebar and the route content, then provided
// via `AGENT_EDIT_KEY` so every tab and the shared sidebar mutate the
// same live instance.
//
// Save payload shape mirrors the legacy monolithic form: kb_scope,
// tools, autonomy, preset_overrides, detection are all embedded directly
// on the agent record so one save action covers every tab. No per-tab
// partial saves.

import { computed, onMounted, onUnmounted, ref, toValue } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import api from '@/api'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'

export const AGENT_EDIT_KEY = Symbol('AgentEdit')

const emptyKBScope = { folder_ids: [], article_ids: [] }

function asPlainObject(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function normalizeToolRefs(tools) {
  if (!Array.isArray(tools)) return []
  return tools
    .filter((tool) => tool && (tool.kind === 'internal' || tool.kind === 'custom'))
    .map((tool) => {
      const out = {
        kind: tool.kind,
        id: tool.kind === 'internal' ? String(tool.id) : Number(tool.id)
      }
      if (tool.config_override != null) out.config_override = tool.config_override
      return out
    })
}

function buildSavePayload(agent) {
  const presetOverrides = {}
  const reasonPreset = asPlainObject(agent.preset_overrides).reason
  if (reasonPreset) presetOverrides.reason = reasonPreset

  const confidenceThreshold = Number(agent.confidence_threshold)
  const maxIterations = Number(agent.max_iterations)
  const verificationFloor = Number(agent.min_verification ?? 0)

  return {
    name: agent.name,
    slug: agent.slug,
    description: agent.description || '',
    prompt: agent.prompt || '',
    kb_scope: {
      folder_ids: (agent.kb_scope?.folder_ids ?? emptyKBScope.folder_ids).map(Number),
      article_ids: (agent.kb_scope?.article_ids ?? emptyKBScope.article_ids).map(Number)
    },
    detection: asPlainObject(agent.detection),
    tools: normalizeToolRefs(agent.tools),
    autonomy: asPlainObject(agent.autonomy),
    preset_overrides: presetOverrides,
    confidence_threshold: Number.isFinite(confidenceThreshold) ? confidenceThreshold : 0.5,
    extended_thinking_enabled: agent.extended_thinking_enabled !== false,
    max_iterations: Number.isFinite(maxIterations) ? maxIterations : 5,
    min_verification: Number.isFinite(verificationFloor) ? verificationFloor : 0,
    enabled: !!agent.enabled
  }
}

function createEmptyAgent() {
  return {
    id: null,
    name: '',
    slug: '',
    description: '',
    prompt: '',
    detection: {},
    tools: [],
    kb_scope: { folder_ids: [], article_ids: [] },
    preset_overrides: {},
    confidence_threshold: 0.5,
    max_iterations: 5,
    extended_thinking_enabled: true,
    min_verification: 0,
    autonomy: {},
    enabled: false
  }
}

export function useAgentEdit(agentId) {
  const emitter = useEmitter()
  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()

  const agent = ref(null)
  const original = ref(null)
  const loading = ref(false)
  const saving = ref(false)
  const loadError = ref(null)

  const rawAgentID = computed(() => route.params.id ?? toValue(agentId))
  const resolveAgentID = () => {
    const id = Number(rawAgentID.value)
    return Number.isFinite(id) && id > 0 ? id : null
  }
  const isCreateMode = computed(() => !resolveAgentID())

  // Deep clone via JSON round-trip. The agent payload is JSON-only —
  // no Date instances, no circular refs — so this is the simplest
  // correct snapshot.
  const clone = (v) => (v == null ? null : JSON.parse(JSON.stringify(v)))

  const isDirty = computed(() => {
    if (!agent.value || !original.value) return false
    return JSON.stringify(agent.value) !== JSON.stringify(original.value)
  })

  async function load() {
    const id = resolveAgentID()
    if (!id) {
      const draft = createEmptyAgent()
      agent.value = draft
      original.value = clone(draft)
      loadError.value = null
      loading.value = false
      return
    }

    if (agent.value?.id && Number(agent.value.id) === id) {
      original.value = clone(agent.value)
      loadError.value = null
      return
    }

    loading.value = true
    loadError.value = null
    try {
      const resp = await api.getAIAgent(id)
      const fresh = resp.data.data
      agent.value = fresh
      original.value = clone(fresh)
    } catch (err) {
      loadError.value = handleHTTPError(err).message
    } finally {
      loading.value = false
    }
  }

  async function save() {
    const id = resolveAgentID()
    if (!agent.value || saving.value) return
    saving.value = true
    try {
      // Rebuild the form payload explicitly so nested JSON fields
      // like kb_scope and tools land in the exact shape the backend
      // expects, instead of trusting the live reactive record blindly.
      const payload = buildSavePayload(agent.value)
      if (id) {
        await api.updateAIAgent(id, payload)
        // Snapshot after success so isDirty reads false until the next
        // edit. Keep agent.value as-is (don't re-fetch) so the UI
        // doesn't flicker.
        agent.value = { ...agent.value, ...clone(payload), id }
        original.value = clone(agent.value)
        emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
          description: t('globals.messages.savedSuccessfully')
        })
        return
      }

      const resp = await api.createAIAgent(payload)
      const created = resp.data?.data
      agent.value = created
      original.value = clone(agent.value)
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        description: t('admin.ai.agent.created')
      })
      if (created?.id) {
        await router.replace({ name: 'edit-ai-agent-identity', params: { id: created.id } })
      }
    } catch (err) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: handleHTTPError(err).message
      })
    } finally {
      saving.value = false
    }
  }

  // Native browser-close guard. Setting returnValue triggers the
  // browser's built-in "leave site?" dialog. Listener registered /
  // unregistered around the layout's lifetime so it doesn't fire on
  // unrelated pages.
  function onBeforeUnload(e) {
    if (isDirty.value) {
      e.preventDefault()
      e.returnValue = ''
    }
  }

  // Cmd+S / Ctrl+S — saves when dirty, preventDefault to avoid the
  // browser's save-page dialog regardless.
  function onKeyDown(e) {
    const isSaveCombo = (e.metaKey || e.ctrlKey) && e.key === 's'
    if (!isSaveCombo) return
    e.preventDefault()
    if (isDirty.value) save()
  }

  onMounted(() => {
    window.addEventListener('beforeunload', onBeforeUnload)
    window.addEventListener('keydown', onKeyDown)
  })
  onUnmounted(() => {
    window.removeEventListener('beforeunload', onBeforeUnload)
    window.removeEventListener('keydown', onKeyDown)
  })

  // Helper for the route-leave guard. Tab switches inside the editor do
  // not prompt; leaving the editor still asks the operator to confirm.
  // Returns true when the navigation is OK, false to cancel.
  function confirmDiscardIfDirty() {
    if (!isDirty.value) return true
    return window.confirm(t('admin.ai.agent.unsavedChanges'))
  }

  return {
    agent,
    original,
    isDirty,
    loading,
    saving,
    loadError,
    isCreateMode,
    load,
    save,
    confirmDiscardIfDirty
  }
}
