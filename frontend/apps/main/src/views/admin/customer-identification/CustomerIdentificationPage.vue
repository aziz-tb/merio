<template>
  <LoadingOverlay :loading="loading" class="h-full">
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div class="sticky top-0 z-10 -mx-1 flex items-center justify-between gap-4 bg-background/95 px-1 py-2 backdrop-blur">
        <div>
          <h1 class="text-xl font-semibold text-foreground">
            {{ t('admin.customerIdentification.title') }}
          </h1>
        </div>
        <Button type="button" :is-loading="saving" @click="save">
          <FloppyDisk class="size-4" />
          {{ t('admin.customerIdentification.save') }}
        </Button>
      </div>

      <Alert v-if="loadError" variant="destructive">
        <AlertDescription>{{ loadError }}</AlertDescription>
      </Alert>

      <section-card
        :title="t('admin.customerIdentification.section.patternIdentification')"
        :description="t('admin.customerIdentification.section.patternIdentificationDesc')"
      >
        <div class="space-y-5" :class="{ 'opacity-60': !form.pattern_detection_enabled }">
          <div class="flex items-center justify-between gap-4 rounded-md border border-input p-4">
            <div>
              <Label>{{ t('admin.customerIdentification.field.patternDetectionEnabled') }}</Label>
            </div>
            <Switch v-model:checked="form.pattern_detection_enabled" />
          </div>

          <div class="grid gap-5 md:grid-cols-2">
            <div class="space-y-2">
              <Label>{{ t('admin.customerIdentification.field.contractNumberRegex') }}</Label>
              <Input v-model="form.contract_number_regex" placeholder="(?:vertrag|contract)\\s*#?\\s*(\\d+)" @keydown.enter.prevent />
              <p class="text-xs text-muted-foreground">
                {{ t('admin.customerIdentification.field.regexHelp') }}
              </p>
              <p v-if="errors.contract_number_regex" class="text-xs text-destructive">
                {{ errors.contract_number_regex }}
              </p>
            </div>

            <div class="space-y-2">
              <Label>{{ t('admin.customerIdentification.field.customerNumberRegex') }}</Label>
              <Input v-model="form.customer_number_regex" placeholder="(?:kunde|customer)\\s*#?\\s*(\\d+)" @keydown.enter.prevent />
              <p class="text-xs text-muted-foreground">
                {{ t('admin.customerIdentification.field.regexHelp') }}
              </p>
              <p v-if="errors.customer_number_regex" class="text-xs text-destructive">
                {{ errors.customer_number_regex }}
              </p>
            </div>
          </div>

          <div class="space-y-2">
            <Label>{{ t('admin.customerIdentification.field.excludedEmailDomains') }}</Label>
            <Input v-model="form.excluded_email_domains_text" placeholder="rabot.energy, rabot-charge.de" @keydown.enter.prevent />
            <p class="text-xs text-muted-foreground">
              {{ t('admin.customerIdentification.field.excludedEmailDomainsHelp') }}
            </p>
          </div>
        </div>
      </section-card>

      <section-card
        :title="t('admin.customerIdentification.section.aiIdentification')"
        :description="t('admin.customerIdentification.section.aiIdentificationDesc')"
      >
        <div class="space-y-5" :class="{ 'opacity-60': !form.llm_extraction_enabled }">
          <div class="flex items-center justify-between gap-4 rounded-md border border-input p-4">
            <div>
              <Label>{{ t('admin.customerIdentification.field.llmExtractionEnabled') }}</Label>
            </div>
            <Switch v-model:checked="form.llm_extraction_enabled" />
          </div>

          <SearchParametersEditor
            v-model="form.search_parameters"
            :errors="errors.search_parameters"
          />
        </div>
      </section-card>

      <section-card
        :title="t('admin.customerIdentification.section.verificationRules')"
        :description="t('admin.customerIdentification.section.verificationRulesDesc')"
      >
        <VerificationRulesEditor
          v-model="form.groups"
          :standard-criteria="STANDARD_CRITERIA"
          :group-errors="errors.groups"
          :custom-errors="errors.custom_criteria"
          :warnings="unknownCriterionWarnings"
        />
      </section-card>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Alert, AlertDescription } from '@shared-ui/components/ui/alert'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Switch } from '@shared-ui/components/ui/switch'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { useUserStore } from '@/stores/user'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import SearchParametersEditor from './SearchParametersEditor.vue'
import VerificationRulesEditor from './VerificationRulesEditor.vue'
import { FloppyDisk } from '@qall/icons'

const PREFIX = 'customer_identification.'
const STANDARD_CRITERIA = [
  'sender_email_matches_erp',
  'mentioned_email_matches_erp',
  'phone_matches_erp',
  'contract_id_mentioned',
  'customer_id_mentioned',
  'firstname_mentioned',
  'lastname_mentioned',
  'exact_delivery_address_mentioned',
  'exact_billing_address_mentioned',
  'similar_delivery_address_mentioned',
  'similar_billing_address_mentioned'
]
const STANDARD_CRITERIA_SET = new Set(STANDARD_CRITERIA)

const SectionCard = {
  props: {
    title: { type: String, required: true },
    description: { type: String, required: true }
  },
  setup(props, { slots }) {
    return () => h('section', { class: 'rounded-lg border border-input bg-background p-5' }, [
      h('div', { class: 'mb-5 border-b border-border pb-4' }, [
        h('h2', { class: 'text-base font-semibold text-foreground' }, props.title),
        h('p', { class: 'mt-1 text-sm text-muted-foreground' }, props.description)
      ]),
      slots.default?.()
    ])
  }
}

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const userStore = useUserStore()

const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const originalSnapshot = ref('')

const form = reactive(defaultForm())
const errors = reactive(emptyErrors())

const unknownCriterionWarnings = computed(() => {
  const warnings = []
  for (const group of form.groups) {
    for (const criterion of group.unknown_criteria || []) {
      warnings.push({ groupId: group.id, criterion })
    }
  }
  return warnings
})

function defaultForm() {
  return {
    pattern_detection_enabled: true,
    contract_number_regex: '',
    customer_number_regex: '',
    llm_extraction_enabled: true,
    erp_cache_ttl_minutes: 60,
    excluded_email_domains_text: '',
    search_parameters: [],
    groups: []
  }
}

function emptyErrors() {
  return {
    contract_number_regex: '',
    customer_number_regex: '',
    search_parameters: {},
    groups: {},
    custom_criteria: {}
  }
}

function resetErrors() {
  Object.assign(errors, emptyErrors())
}

function makeID(prefix) {
  if (window.crypto?.randomUUID) return window.crypto.randomUUID()
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function assignForm(next) {
  Object.assign(form, defaultForm(), next)
}

function normalizeSettings(data = {}) {
  const rules = data.verification_rules_async || {}
  const email = rules.email || {}
  return {
    pattern_detection_enabled: Boolean(data.pattern_detection_enabled),
    contract_number_regex: data.contract_number_regex || '',
    customer_number_regex: data.customer_number_regex || '',
    llm_extraction_enabled: Boolean(data.llm_extraction_enabled),
    erp_cache_ttl_minutes: Number(data.erp_cache_ttl_minutes ?? 60),
    excluded_email_domains_text: normalizeDomains(data.excluded_email_domains).join(', '),
    search_parameters: searchParametersToRows(data.search_parameters || {}),
    groups: normalizeGroups(email.groups || [])
  }
}

function normalizeDomains(value) {
  return Array.isArray(value) ? value.filter((item) => typeof item === 'string') : []
}

function searchParametersToRows(value) {
  return Object.entries(value || {}).map(([key, config]) => ({
    id: makeID('search-param'),
    key,
    datatype: config?.datatype || 'string',
    description: config?.description || ''
  }))
}

function normalizeGroups(groups) {
  return groups.map((group) => {
    const standardCriteria = []
    const unknownCriteria = []
    for (const criterion of group.criteria || []) {
      const name = typeof criterion === 'string' ? criterion : criterion?.name
      const type = typeof criterion === 'string' ? 'standard' : criterion?.type
      if (type === 'standard' && name) {
        if (STANDARD_CRITERIA_SET.has(name)) {
          standardCriteria.push(name)
        } else {
          unknownCriteria.push(name)
        }
      }
    }
    return {
      id: makeID('group'),
      label: group.label || '',
      min_count: Number(group.min_count || 1),
      standard_criteria: dedupe(standardCriteria),
      unknown_criteria: dedupe(unknownCriteria),
      custom_criteria: (group.custom_criteria || []).map((criterion) => ({
        id: makeID('custom-criterion'),
        label: criterion.label || '',
        key: criterion.key || '',
        reason: criterion.reason || '',
        search_in: Array.isArray(criterion.search_in) ? criterion.search_in : [],
        open: false
      })),
      open: false
    }
  })
}

function dedupe(items) {
  return [...new Set(items.filter(Boolean))]
}

function domainsFromText(value) {
  return dedupe(
    String(value || '')
      .split(',')
      .map((domain) => domain.trim())
      .filter(Boolean)
  )
}

function rowsToSearchParameters(rows) {
  return rows.reduce((acc, row) => {
    const key = row.key.trim()
    if (!key) return acc
    acc[key] = {
      datatype: row.datatype || 'string',
      description: row.description || ''
    }
    return acc
  }, {})
}

function serializeGroups(groups) {
  return groups.map((group) => {
    const standardRefs = dedupe([
      ...(group.standard_criteria || []),
      ...(group.unknown_criteria || [])
    ]).map((name) => ({ type: 'standard', name }))
    const customCriteria = (group.custom_criteria || []).map((criterion) => ({
      label: criterion.label.trim(),
      key: criterion.key.trim(),
      reason: criterion.reason.trim(),
      search_in: dedupe(criterion.search_in || [])
    }))
    const customRefs = customCriteria
      .filter((criterion) => criterion.label)
      .map((criterion) => ({ type: 'custom', label: criterion.label }))
    return {
      label: group.label.trim(),
      min_count: Number(group.min_count || 1),
      criteria: [...standardRefs, ...customRefs],
      custom_criteria: customCriteria
    }
  })
}

function settingsForSave() {
  return {
    pattern_detection_enabled: Boolean(form.pattern_detection_enabled),
    contract_number_regex: form.contract_number_regex || '',
    customer_number_regex: form.customer_number_regex || '',
    llm_extraction_enabled: Boolean(form.llm_extraction_enabled),
    erp_cache_ttl_minutes: Number(form.erp_cache_ttl_minutes ?? 60),
    excluded_email_domains: domainsFromText(form.excluded_email_domains_text),
    search_parameters: rowsToSearchParameters(form.search_parameters),
    verification_rules_async: {
      email: {
        groups: serializeGroups(form.groups)
      }
    }
  }
}

function prefixedPayload(settings) {
  return Object.fromEntries(
    Object.entries(settings).map(([key, value]) => [`${PREFIX}${key}`, value])
  )
}

function snapshot(settings) {
  return JSON.stringify(settings)
}

function validate() {
  resetErrors()
  let ok = true
  for (const key of ['contract_number_regex', 'customer_number_regex']) {
    try {
      if (String(form[key] || '').trim()) new RegExp(form[key])
    } catch {
      errors[key] = t('admin.customerIdentification.validation.invalidRegex')
      ok = false
    }
  }

  const seenSearchKeys = new Set()
  for (const row of form.search_parameters) {
    const key = row.key.trim()
    if (!key) {
      errors.search_parameters[row.id] = t('admin.customerIdentification.validation.emptyKey')
      ok = false
    } else if (/\s/.test(key)) {
      errors.search_parameters[row.id] = t('admin.customerIdentification.validation.keyNoWhitespace')
      ok = false
    } else if (seenSearchKeys.has(key)) {
      errors.search_parameters[row.id] = t('admin.customerIdentification.validation.duplicateKey')
      ok = false
    }
    seenSearchKeys.add(key)
    if (!row.datatype) {
      errors.search_parameters[row.id] = t('admin.customerIdentification.validation.datatypeRequired')
      ok = false
    }
  }

  for (const group of form.groups) {
    if (!group.label.trim()) {
      errors.groups[group.id] = t('admin.customerIdentification.validation.groupLabelRequired')
      group.open = true
      ok = false
    } else if (Number(group.min_count) < 1) {
      errors.groups[group.id] = t('admin.customerIdentification.validation.minCount')
      group.open = true
      ok = false
    }

    const labels = new Set()
    for (const criterion of group.custom_criteria) {
      const label = criterion.label.trim()
      let err = ''
      if (!label) err = t('admin.customerIdentification.validation.customLabelRequired')
      else if (labels.has(label)) err = t('admin.customerIdentification.validation.customLabelUnique')
      else if (!criterion.key.trim()) err = t('admin.customerIdentification.validation.customKeyRequired')
      else if (!criterion.reason.trim()) err = t('admin.customerIdentification.validation.customReasonRequired')
      else if (!Array.isArray(criterion.search_in) || criterion.search_in.length === 0) {
        err = t('admin.customerIdentification.validation.customSearchInRequired')
      }
      labels.add(label)
      if (err) {
        errors.custom_criteria[criterion.id] = err
        group.open = true
        criterion.open = true
        ok = false
      }
    }
  }
  return ok
}

async function ensurePermission() {
  if (!userStore.userID) {
    await userStore.getCurrentUser()
  }
  if (!userStore.can('customers:manage')) {
    router.replace('/admin')
    return false
  }
  return true
}

async function loadSettings() {
  if (!(await ensurePermission())) return
  loading.value = true
  loadError.value = ''
  try {
    const resp = await api.getCustomerIdentificationSettings()
    const normalized = normalizeSettings(resp.data.data || {})
    assignForm(normalized)
    originalSnapshot.value = snapshot(settingsForSave())
  } catch (error) {
    loadError.value = handleHTTPError(error).message
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: loadError.value
    })
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!validate()) return
  const settings = settingsForSave()
  const nextSnapshot = snapshot(settings)
  if (nextSnapshot === originalSnapshot.value) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.customerIdentification.nothingToSave')
    })
    return
  }
  saving.value = true
  try {
    await api.updateCustomerIdentificationSettings(prefixedPayload(settings))
    originalSnapshot.value = nextSnapshot
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.customerIdentification.saved')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>
