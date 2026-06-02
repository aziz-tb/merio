<template>
  <PageHeader :description="t('admin.customerIdentification.erpConnectorsIntro')" />
  <LoadingOverlay :loading="loading" class="h-full">
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <Alert v-if="loadError" variant="destructive">
        <AlertDescription>{{ loadError }}</AlertDescription>
      </Alert>

      <div class="space-y-4">
        <div
          v-for="fn in FUNCTIONS"
          :key="fn.name"
          class="rounded-lg border border-input bg-background"
        >
          <Collapsible :open="openState[fn.name]" @update:open="(v) => setOpen(fn.name, v)">
            <CollapsibleTrigger as-child>
              <button type="button" class="flex w-full items-start justify-between gap-4 p-4 text-left">
                <div class="flex min-w-0 gap-3">
                  <ChevronRight
                    :class="[
                      'mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform',
                      openState[fn.name] ? 'rotate-90' : ''
                    ]"
                  />
                  <div class="min-w-0">
                    <h2 class="text-base font-semibold text-foreground">
                      {{ t(fn.labelKey) }}
                    </h2>
                    <p class="mt-1 text-sm text-muted-foreground">
                      {{ t(fn.descriptionKey) }}
                    </p>
                  </div>
                </div>
              </button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div class="border-t border-border p-4">
                <FunctionEditor :name="fn.name" />
              </div>
            </CollapsibleContent>
          </Collapsible>
        </div>
      </div>

      <div class="rounded-lg border border-input bg-background p-5">
        <div class="mb-5 border-b border-border pb-4">
          <h2 class="text-base font-semibold text-foreground">
            {{ t('admin.customerIdentification.cache.title') }}
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-muted-foreground">
            {{ t('admin.customerIdentification.cache.description') }}
          </p>
        </div>

        <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div class="space-y-2 md:max-w-xs">
            <Label>{{ t('admin.customerIdentification.cache.ttl') }}</Label>
            <div class="flex items-center gap-2">
              <Input
                type="number"
                min="0"
                step="1"
                :model-value="cacheTtl"
                @input="(e) => setCacheTtl(e.target.value)"
              />
              <span class="text-sm text-muted-foreground">
                {{ t('admin.customerIdentification.cache.minutes') }}
              </span>
            </div>
          </div>

          <Button type="button" :is-loading="savingCache" @click="saveCache">
            <FloppyDisk class="size-4" />
            {{ t('admin.customerIdentification.save') }}
          </Button>
        </div>
      </div>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Alert, AlertDescription } from '@shared-ui/components/ui/alert'
import { Button } from '@shared-ui/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '@shared-ui/components/ui/collapsible'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import LoadingOverlay from '@/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import FunctionEditor from '@/features/ai/functions/FunctionEditor.vue'
import { useEmitter } from '@/composables/useEmitter'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { useUserStore } from '@/stores/user'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { FloppyDisk, ChevronRight } from '@qall/icons'

const FUNCTIONS = [
  {
    name: 'search_customer_by_fields',
    labelKey: 'admin.customerIdentification.function.searchCustomerByFields.label',
    descriptionKey: 'admin.customerIdentification.function.searchCustomerByFields.description'
  },
  {
    name: 'search_contract_by_id',
    labelKey: 'admin.customerIdentification.function.searchContractById.label',
    descriptionKey: 'admin.customerIdentification.function.searchContractById.description'
  },
  {
    name: 'search_customer_by_id',
    labelKey: 'admin.customerIdentification.function.searchCustomerById.label',
    descriptionKey: 'admin.customerIdentification.function.searchCustomerById.description'
  },
  {
    name: 'search_customer_by_free_text',
    labelKey: 'admin.customerIdentification.function.searchCustomerByFreeText.label',
    descriptionKey: 'admin.customerIdentification.function.searchCustomerByFreeText.description'
  }
]

const { t } = useI18n()
const router = useRouter()
const emitter = useEmitter()
const userStore = useUserStore()

const loading = ref(false)
const loadError = ref('')
// All function blocks start expanded; operators collapse individual blocks
// via the chevron. Keyed by function name.
const openState = ref(Object.fromEntries(FUNCTIONS.map((fn) => [fn.name, true])))
const cacheTtl = ref(60)
const savingCache = ref(false)

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

async function load() {
  if (!(await ensurePermission())) return
  loading.value = true
  loadError.value = ''
  try {
    const settingsResp = await api.getCustomerIdentificationSettings()
    cacheTtl.value = Number(settingsResp.data.data?.erp_cache_ttl_minutes ?? 60)
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

function setOpen(name, open) {
  openState.value[name] = open
}

function setCacheTtl(value) {
  const next = Number(value)
  cacheTtl.value = Number.isFinite(next) ? next : 0
}

async function saveCache() {
  savingCache.value = true
  try {
    await api.updateCustomerIdentificationSettings({
      erp_cache_ttl_minutes: Number(cacheTtl.value)
    })
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('admin.customerIdentification.saved')
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    savingCache.value = false
  }
}

onMounted(load)
</script>
