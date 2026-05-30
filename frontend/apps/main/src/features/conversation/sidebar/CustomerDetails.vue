<template>
  <Collapsible defaultOpen class="group/section border-t border-border pt-3">
    <CollapsibleTrigger class="flex items-center justify-between w-full text-sm font-semibold py-1 transition-colors hover:text-primary">
      <span class="flex items-center gap-2">
        <component :is="stateIcon" :class="['size-4']" />
        {{ t('conversation.contactData.title') }}
      </span>
      <ChevronDown class="size-4 text-muted-foreground transition-transform group-data-[state=closed]/section:-rotate-90" />
    </CollapsibleTrigger>
    <CollapsibleContent class="pt-3">
      <div v-if="!customer" class="mb-3 rounded-md border border-input bg-background p-3">
        <div class="flex items-start justify-between gap-2">
          <div class="flex min-w-0 items-start gap-2">
            <Avatar class="mt-1 size-8">
              <AvatarImage v-if="contactAvatar" :src="contactAvatar" :alt="contactName" />
              <AvatarFallback>
                <User class="size-4 text-muted-foreground" />
              </AvatarFallback>
            </Avatar>
            <div class="min-w-0 flex-1">
              <template v-if="!customer">
                <router-link
                  v-if="canOpenContact"
                  :to="{ name: 'contact-detail', params: { id: conversation.contact_id } }"
                  class="inline-flex max-w-full min-w-0 items-center gap-1"
                >
                  <div class="min-w-0">
                    <p class="flex truncate text-sm font-medium text-foreground decoration-1 underline-offset-2 hover:underline">
                      {{ contactName }}
                      <ExternalLink class="size-2.5 ml-2 mt-1 shrink-0 text-muted-foreground" />
                    </p>
                    <p class="truncate text-xs text-muted-foreground decoration-1 underline-offset-2 hover:underline">
                      {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                    </p>
                  </div>
                  
                </router-link>
                <div v-else class="min-w-0">
                  <p class="truncate text-xs text-muted-foreground">
                    {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                  </p>
                </div>
              </template>

              <template v-else>
                <router-link
                  v-if="canOpenContact"
                  :to="{ name: 'contact-detail', params: { id: conversation.contact_id } }"
                  class="group/contact mt-2 inline-flex max-w-full min-w-0 items-center gap-1"
                >
                  <p class="truncate text-xs font-medium text-muted-foreground decoration-1 underline-offset-2 group-hover/contact:underline">
                    {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                  </p>
                  <ExternalLink class="size-2.5 ml-2 shrink-0 text-muted-foreground" />
                </router-link>
                <div v-else class="min-w-0">
                  <p class="mt-2 truncate text-xs font-medium text-muted-foreground">
                    {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                  </p>
                </div>
              </template>
            </div>
          </div>
        </div>

        <Collapsible v-if="hasContactDetails" class="group/contactDetails mt-3 border-t border-border pt-2">
          <CollapsibleTrigger class="flex w-full items-center justify-between text-xs font-medium text-muted-foreground transition-colors hover:text-foreground">
            <span>{{ t('conversation.contactData.moreDetails') }}</span>
            <ChevronDown class="size-3.5 transition-transform group-data-[state=closed]/contactDetails:-rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent class="mt-2 space-y-2 text-xs">
            <div v-if="phoneNumber" class="flex items-center gap-2">
              <Phone class="size-3.5 shrink-0 text-muted-foreground" />
              <span class="min-w-0 truncate text-foreground">{{ phoneNumber }}</span>
            </div>
            <div v-if="stringValue(contact.external_user_id)" class="flex items-center gap-2">
              <IdCard class="size-3.5 shrink-0 text-muted-foreground" />
              <span class="min-w-0 truncate text-foreground">{{ stringValue(contact.external_user_id) }}</span>
            </div>
            <template v-if="isLivechat">
              <div v-if="countryName" class="flex items-center gap-2">
                <Globe class="size-3.5 shrink-0 text-muted-foreground" />
                <span class="min-w-0 truncate text-foreground">{{ countryName }}</span>
              </div>
              <div v-if="conversation.meta?.ip" class="flex items-center gap-2">
                <Monitor class="size-3.5 shrink-0 text-muted-foreground" />
                <span class="min-w-0 truncate text-foreground">{{ conversation.meta.ip }}</span>
              </div>
              <div v-if="parsedUA" class="flex items-center gap-2">
                <Smartphone class="size-3.5 shrink-0 text-muted-foreground" />
                <span class="min-w-0 truncate text-foreground">{{ parsedUA }}</span>
              </div>
            </template>
          </CollapsibleContent>
        </Collapsible>
      </div>

      <ConflictInfoPanel
        v-if="conflictNotice && !conflictDismissed"
        class="mb-3"
        :previous-id="conflictNotice.previousId"
        :current-id="conflictNotice.currentId"
        @dismiss="dismissConflictNotice"
      />

      <div v-else-if="showSearchState" class="space-y-3">
        <div
          v-if="running"
          class="rounded-md border border-input bg-muted/20 px-4 py-8 text-center text-sm text-muted-foreground"
        >
          <Loader2 class="mx-auto mb-2 size-4 animate-spin" />
          {{ t('conversation.customer.searching') }}
        </div>

        <template v-else>
          <Popover v-model:open="dropdownOpen">
            <PopoverAnchor as-child>
              <div ref="searchRow" class="flex items-center gap-1.5">
                <div class="relative min-w-0 flex-1">
                  <Search class="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    v-model="searchQuery"
                    class="h-8 pl-8 text-xs"
                    :placeholder="t('conversation.customer.searchPlaceholder')"
                    autocomplete="off"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="dropdownOpen"
                    @focus="onInputFocus"
                    @keydown.down.prevent="onArrowDown"
                    @keydown.up.prevent="onArrowUp"
                    @keydown.enter.prevent="onEnter"
                    @keydown.esc="closeDropdown"
                  />
                </div>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      class="size-8"
                      :disabled="startingIdentification"
                      @click="reIdentify"
                    >
                      <Loader2 v-if="startingIdentification" class="size-4 animate-spin" />
                      <RefreshCw v-else class="size-4" />
                      <span class="sr-only">{{ t('conversation.customer.reIdentify') }}</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{{ t('conversation.customer.reIdentify') }}</TooltipContent>
                </Tooltip>
              </div>
            </PopoverAnchor>

            <FailurePanel
              v-if="visiblePanel"
              :title="visiblePanel.title"
              :message="visiblePanel.message"
              @dismiss="dismissPanel"
            />
            <ReconnectInfoPanel
              v-if="showReconnectPanel"
              :customer="linkedCustomer"
              @reconnect="reconnectLinkedCustomer"
              @dismiss="dismissReconnectPanel"
            />
            <div v-if="loading" class="rounded-md border border-input bg-muted/20 px-4 py-6 text-center text-sm text-muted-foreground">
              <Loader2 class="mx-auto mb-2 size-4 animate-spin" />
              {{ t('globals.messages.loading') }}
            </div>

            <PopoverContent
              align="start"
              :side-offset="6"
              :trap-focus="false"
              :style="dropdownStyle"
              class="p-0"
              @open-auto-focus.prevent
              @close-auto-focus.prevent
              @interact-outside="onInteractOutside"
            >
              <div ref="dropdownList" class="max-h-72 overflow-y-auto py-1">
                <div v-if="searching" class="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
                  <Loader2 class="size-3.5 animate-spin" />
                  {{ t('conversation.customer.searching') }}
                </div>

                <div v-else-if="searchError" class="px-3 py-2 text-xs text-muted-foreground">
                  {{ t('conversation.customer.searchError') }}
                </div>

                <div v-else-if="searchResults.length === 0" class="px-3 py-2 text-xs text-muted-foreground">
                  {{ t('conversation.customer.noResults') }}
                </div>

                <template v-else>
                  <div v-for="(result, index) in searchResults" :key="result.customer_id" :data-index="index">
                    <button
                      type="button"
                      class="flex w-full items-start justify-between gap-2 px-3 py-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50"
                      :class="index === highlightedIndex ? 'bg-muted' : 'hover:bg-muted'"
                      :disabled="assigning || !contractIdsForResult(result).length"
                      @click="selectSearchResult(result)"
                      @mousemove="highlightedIndex = index"
                    >
                      <span class="min-w-0">
                        <span class="block truncate text-sm font-medium text-foreground">{{ customerName(result) }}</span>
                        <span class="block truncate text-xs text-muted-foreground">
                          {{ result.customer_id }}
                          <template v-if="stringValue(result.email)"> · {{ stringValue(result.email) }}</template>
                        </span>
                        <span v-if="contractSummary(result)" class="block truncate text-xs text-muted-foreground">
                          {{ contractSummary(result) }}
                        </span>
                      </span>
                    </button>
                  </div>
                </template>
              </div>
            </PopoverContent>
          </Popover>
        </template>
      </div>

      <div v-else class="space-y-3 rounded-md border border-input bg-background p-3">
        <div class="flex items-start justify-between gap-2">

          <div class="min-w-0">
            <div class="flex min-w-0 items-start gap-2">
              <Avatar class="mt-1 size-7">
                <AvatarImage v-if="contactAvatar" :src="contactAvatar" :alt="contactName" />
                <AvatarFallback>
                  <User class="size-3 text-muted-foreground" />
                </AvatarFallback>
              </Avatar>
              <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-semibold -mb-1">
                <span v-if="isPartial" class="text-muted-foreground">{{ t('conversation.customer.maybe') }} </span>
                {{ customerName(customer) }}
              </p>
              <template v-if="customer">
                <router-link
                  v-if="canOpenContact"
                  :to="{ name: 'contact-detail', params: { id: conversation.contact_id } }"
                  class="group/contact inline-flex max-w-full min-w-0 items-center gap-1"
                >
                  <p class="truncate text-xs font-medium text-muted-foreground decoration-1 underline-offset-2 group-hover/contact:underline">
                    {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                  </p>
                  <ExternalLink class="size-2.5 ml-2 shrink-0 text-muted-foreground" />
                </router-link>
                <div v-else class="min-w-0">
                  <p class=" truncate text-xs font-medium text-muted-foreground">
                    {{ contactEmail || t('conversation.sidebar.notAvailable') }}
                  </p>
                </div>
              </template>
              </div>
          </div>
        </div>

          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <Button type="button" size="icon" variant="ghost" class="size-7 shrink-0">
                <MoreVertical class="size-4" />
                <span class="sr-only">{{ t('globals.terms.action', 2) }}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem @click="showSearchInstead">
                {{ t('conversation.customer.menu.searchNew') }}
              </DropdownMenuItem>
              <DropdownMenuItem @click="removeCustomer">
                {{ t('conversation.customer.menu.removeCustomer') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <div v-if="isPartial && stringValue(conversation.verification_message)" class="rounded-md border border-yellow-200 bg-yellow-50 p-2 text-xs text-yellow-900 dark:border-yellow-900/60 dark:bg-yellow-950/40 dark:text-yellow-100">
          <div class="flex items-start gap-2">
            <AlertTriangle class="mt-0.5 size-4 shrink-0" />
            <p class="whitespace-pre-wrap">{{ stringValue(conversation.verification_message) }}</p>
          </div>
        </div>

        <div v-if="contracts.length > 0" class="space-y-1">
          
        <p class="truncate text-xs text-muted-foreground">
          {{ t('conversation.customer.customerNumber') }}: {{ customer?.customer_id || '-' }}
        </p>
        
          <p class="sidebar-label">{{ t('conversation.customer.contract') }}</p>
          <PillSwitcher :model-value="selectedContractID" @update:modelValue="swapContract">
            <PillSwitcherItem
              v-for="item in contracts"
              :key="item.contract_id"
              :value="item.contract_id"
              class="min-w-0 text-xs"
              :disabled="assigning"
            >
              <span class="truncate">{{ item.contract_id }}</span>
            </PillSwitcherItem>
          </PillSwitcher>
        </div>
        <div v-else-if="contract" class="flex items-center justify-between gap-2">
          <p class="text-sm font-medium">
            {{ t('conversation.customer.contract') }} {{ contract.contract_id }}
          </p>
        </div>

        <div v-if="erpLinks.length" class="flex flex-wrap gap-1.5">
          <Button
            v-for="link in erpLinks"
            :key="link.url"
            as="a"
            variant="outline"
            size="sm"
            class="h-7 px-2 text-xs w-full shadow-sm"
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
          >
            <ExternalLink class="size-3.5" />
            {{ link.label || t('conversation.customer.openInERP') }}
          </Button>
        </div>

        <div v-if="visiblePreviewItems.length" class="space-y-2">
          <div v-for="item in visiblePreviewItems" :key="item.key" class="grid grid-cols-2 gap-2 text-xs">
            <p class="truncate text-muted-foreground">{{ item.label }}</p>
            <Tooltip v-if="item.tooltip">
              <TooltipTrigger as-child>
                <p class="break-words text-right text-foreground">{{ item.value || '-' }}</p>
              </TooltipTrigger>
              <TooltipContent>{{ item.tooltip }}</TooltipContent>
            </Tooltip>
            <p v-else class="break-words text-right text-foreground">{{ item.value || '-' }}</p>
          </div>
          <Button
            v-if="previewItems.length > previewLimit"
            type="button"
            variant="ghost"
            size="sm"
            class="mx-auto flex h-7 text-xs"
            @click="previewExpanded = !previewExpanded"
          >
            {{ previewExpanded ? t('conversation.customer.showLess') : t('conversation.customer.showMore') }}
            <ChevronUp v-if="previewExpanded" class="size-3.5" />
            <ChevronDown v-else class="size-3.5" />
          </Button>
        </div>
      </div>
    </CollapsibleContent>
  </Collapsible>
</template>

<script setup>
import { computed, h, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDebounceFn } from '@vueuse/core'
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Loader2,
  Globe,
  IdCard,
  Monitor,
  MoreVertical,
  RefreshCw,
  Search,
  Smartphone,
  X,
} from 'lucide-vue-next'
import { UserQuestionColored, UserCheckColored, User, Phone } from '@qall/icons'

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@shared-ui/components/ui/collapsible'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared-ui/components/ui/dropdown-menu'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { Popover, PopoverAnchor, PopoverContent } from '@shared-ui/components/ui/popover'
import { PillSwitcher, PillSwitcherItem } from '@shared-ui/components/ui/pill-switcher'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'
import { useConversationStore } from '@/stores/conversation'
import { useUserStore } from '@/stores/user'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { useEmitter } from '@/composables/useEmitter'
import { useWorkflowCompletion } from '@/composables/useWorkflowCompletion'
import countries from '@/constants/countries.js'
import ConflictInfoPanel from './ConflictInfoPanel.vue'
import ReconnectInfoPanel from './ReconnectInfoPanel.vue'

const FailurePanel = {
  props: {
    title: { type: String, required: true },
    message: { type: String, required: false, default: '' },
  },
  emits: ['dismiss'],
  setup (props, { emit }) {
    return () => h('div', { class: 'rounded-md border border-yellow-200 bg-yellow-50 p-2 text-xs text-yellow-900 dark:border-yellow-900/60 dark:bg-yellow-950/40 dark:text-yellow-100' }, [
      h('div', { class: 'flex items-start justify-between gap-2' }, [
        h('div', { class: 'flex min-w-0 gap-2' }, [
          h(AlertTriangle, { class: 'mt-0.5 size-4 shrink-0' }),
          h('div', { class: 'min-w-0' }, [
            h('p', { class: 'font-medium' }, props.title),
            props.message ? h('p', { class: 'mt-1 whitespace-pre-wrap break-words' }, props.message) : null,
          ]),
        ]),
        h('button', {
          type: 'button',
          class: 'rounded p-0.5 text-yellow-800 hover:bg-yellow-100 dark:text-yellow-100 dark:hover:bg-yellow-900/40',
          onClick: () => emit('dismiss'),
        }, [h(X, { class: 'size-3.5' })]),
      ]),
    ])
  },
}

const conversationStore = useConversationStore()
const userStore = useUserStore()
const emitter = useEmitter()
const { t } = useI18n()

const loading = ref(false)
const customer = ref(null)
const contract = ref(null)
const linkedCustomer = ref(null)
const latestOperation = ref(null)
const searchMode = ref(false)
const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)
const searchError = ref(false)
const dropdownOpen = ref(false)
const highlightedIndex = ref(-1)
const dropdownWidth = ref(0)
const searchRow = ref(null)
const dropdownList = ref(null)
const assigning = ref(false)
const running = ref(false)
const startingIdentification = ref(false)
const timeoutPanel = ref(false)
const dismissedTraceID = ref('')
const reconnectDismissed = ref(false)
const conflictDismissed = ref(false)
const conflictNotice = ref(null)
const previewExpanded = ref(false)
const previewLimit = 7
let runningTimeout = null

const SEARCH_MIN_CHARS = 2
const SEARCH_DEBOUNCE_MS = 300
const SEARCH_MAX_RESULTS = 10
let searchAbort = null
let searchSeq = 0

const conversation = computed(() => conversationStore.current || {})
const contact = computed(() => conversation.value?.contact || {})
const uuid = computed(() => conversation.value?.uuid || '')
const contactEmail = computed(() => stringValue(contact.value?.email))
const contactAvatar = computed(() => stringValue(contact.value?.avatar_url))
const canOpenContact = computed(() => !!conversation.value?.contact_id && userStore.can('contacts:read'))
const contactName = computed(() => {
  const first = stringValue(contact.value?.first_name)
  const last = stringValue(contact.value?.last_name)
  return [first, last].filter(Boolean).join(' ') || contactEmail.value || '-'
})
const phoneNumber = computed(() => {
  const countryCodeValue = stringValue(contact.value?.phone_number_country_code)
  const number = stringValue(contact.value?.phone_number)
  if (!number) return ''
  if (!countryCodeValue) return number
  const country = countries.find((c) => c.iso_2 === countryCodeValue)
  const callingCode = country ? country.calling_code : countryCodeValue
  return `${callingCode} ${number}`
})
const countryName = computed(() => {
  const code = stringValue(contact.value?.country)
  if (!code) return ''
  const c = countries.find((c) => c.iso_2 === code)
  return c ? c.name : code
})
const isLivechat = computed(() => conversation.value?.inbox_channel === 'livechat')
const parsedUA = computed(() => {
  const ua = conversation.value?.meta?.user_agent
  if (!ua) return ''
  const browser = ua.match(/(Chrome|Firefox|Safari|Edge|Opera|MSIE|Trident)[/\s](\d+)/i)
  const os = ua.match(/(Windows|Mac OS X|Linux|Android|iOS|iPhone|iPad)[\s/]?([0-9._]*)/i)
  const parts = []
  if (browser) parts.push(`${browser[1]} ${browser[2]}`)
  if (os) parts.push(os[1].replace('_', ' '))
  return parts.length > 0 ? parts.join(' / ') : ua.substring(0, 60)
})
const verificationLevel = computed(() => Number(conversation.value?.verification_level || 0))
const isPartial = computed(() => verificationLevel.value > 0 && verificationLevel.value < 2)
const showSearchState = computed(() => searchMode.value || verificationLevel.value <= 0 || !customer.value)
const contracts = computed(() => Array.isArray(customer.value?.contracts) ? customer.value.contracts : [])
const selectedContractID = computed(() => contract.value?.contract_id || conversation.value?.contract_id || null)
const erpLinks = computed(() => normalizeLinks(contract.value?.erp_urls))
const previewItems = computed(() => normalizePreview(contract.value?.agent_preview))
const visiblePreviewItems = computed(() => previewExpanded.value ? previewItems.value : previewItems.value.slice(0, previewLimit))
const dropdownStyle = computed(() => (dropdownWidth.value ? { width: `${dropdownWidth.value}px` } : {}))
const hasContactDetails = computed(() => {
  return Boolean(
    phoneNumber.value ||
    stringValue(contact.value?.external_user_id) ||
    (isLivechat.value && (countryName.value || conversation.value?.meta?.ip || parsedUA.value))
  )
})
const latestOperationNoMatch = computed(() => {
  const op = latestOperation.value
  if (!op || op.operation_type !== 'customer_identification' || op.status !== 'success') return false
  if (op.payload?.method === 'manual') return false
  return !op.payload?.result?.customer_id
})
const showReconnectPanel = computed(() => {
  return verificationLevel.value === 0 &&
    Boolean(contact.value?.linked_customer_id) &&
    latestOperationNoMatch.value &&
    linkedCustomer.value &&
    !reconnectDismissed.value
})

const stateIcon = computed(() => {
  if (verificationLevel.value >= 2 && !searchMode.value) return UserCheckColored
  if (verificationLevel.value > 0 && !searchMode.value) return UserQuestionColored
  return User
})

const visiblePanel = computed(() => {
  if (timeoutPanel.value) {
    return {
      title: t('conversation.customer.lastAttemptTimedOut'),
      message: '',
    }
  }
  const op = latestOperation.value
  if (!op || op.status !== 'failed' || dismissedTraceID.value === op.trace_id) return null
  return {
    title: t('conversation.customer.lastAttemptFailed'),
    message: stringValue(op.error_message) || stringValue(op.payload?.error) || '',
  }
})

onUnmounted(() => {
  clearRunningTimeout()
  cancelInFlightSearch()
})

const refresh = async () => {
  if (!uuid.value) return
  loading.value = true
  try {
    const [customerResp, contractResp, latestResp] = await Promise.allSettled([
      api.getConversationCustomer(uuid.value),
      api.getConversationContract(uuid.value),
      api.getLatestCustomerIdentification(uuid.value),
    ])

    const failures = []
    if (customerResp.status === 'rejected') {
      failures.push('customer')
      console.warn('CustomerDetails refresh: customer fetch failed', customerResp.reason)
    }
    if (contractResp.status === 'rejected') {
      failures.push('contract')
      console.warn('CustomerDetails refresh: contract fetch failed', contractResp.reason)
    }
    if (latestResp.status === 'rejected') {
      failures.push('latest operation')
      console.warn('CustomerDetails refresh: latest operation fetch failed', latestResp.reason)
    }
    if (failures.length > 0) {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: t('conversation.customer.refreshFailed'),
      })
    }

    customer.value = customerResp.status === 'fulfilled' ? customerResp.value.data?.data || null : null
    contract.value = contractResp.status === 'fulfilled' ? contractResp.value.data?.data || null : null
    const ops = latestResp.status === 'fulfilled' ? latestResp.value.data?.data || [] : []
    latestOperation.value = Array.isArray(ops) ? ops[0] || null : ops || null
    await loadLinkedCustomerForReconnect()
    if (latestOperation.value?.status === 'running') {
      startRunning()
    }
  } finally {
    loading.value = false
  }
}

async function loadLinkedCustomerForReconnect () {
  linkedCustomer.value = null
  const linkedID = stringValue(contact.value?.linked_customer_id)
  if (verificationLevel.value !== 0 || !linkedID || !latestOperationNoMatch.value) return
  try {
    const resp = await api.getCustomer(linkedID)
    linkedCustomer.value = resp.data?.data || null
  } catch {
    linkedCustomer.value = null
  }
}

const closeDropdown = () => {
  dropdownOpen.value = false
  highlightedIndex.value = -1
}

const measureDropdown = () => {
  dropdownWidth.value = searchRow.value?.offsetWidth || 0
}

const cancelInFlightSearch = () => {
  if (searchAbort) {
    searchAbort.abort()
    searchAbort = null
  }
}

// Drop back to the resting State A: cancel any request, invalidate pending
// responses (bump the sequence), and hide the dropdown.
const resetSearchToIdle = () => {
  cancelInFlightSearch()
  searchSeq += 1
  searchResults.value = []
  searchError.value = false
  searching.value = false
  closeDropdown()
}

const performSearch = async () => {
  const q = searchQuery.value.trim()
  if (q.length < SEARCH_MIN_CHARS) {
    resetSearchToIdle()
    return
  }
  // Cancel the previous request and tag this one so a late, superseded
  // response can't overwrite fresher state.
  cancelInFlightSearch()
  searchAbort = new AbortController()
  const controller = searchAbort
  const seq = (searchSeq += 1)
  searching.value = true
  searchError.value = false
  highlightedIndex.value = -1
  measureDropdown()
  dropdownOpen.value = true
  try {
    const resp = await api.searchCustomers(q, { signal: controller.signal })
    if (seq !== searchSeq) return
    const data = Array.isArray(resp.data?.data) ? resp.data.data : []
    searchResults.value = data.slice(0, SEARCH_MAX_RESULTS)
  } catch (error) {
    // Aborted requests are expected when typing fast — ignore them.
    if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') return
    if (seq !== searchSeq) return
    searchResults.value = []
    searchError.value = true
  } finally {
    if (seq === searchSeq) searching.value = false
  }
}

const debouncedSearch = useDebounceFn(performSearch, SEARCH_DEBOUNCE_MS)

watch(searchQuery, (val) => {
  // Below threshold: close immediately rather than waiting out the debounce.
  if ((val || '').trim().length < SEARCH_MIN_CHARS) {
    resetSearchToIdle()
    return
  }
  debouncedSearch()
})

const onInputFocus = () => {
  if (
    searchQuery.value.trim().length >= SEARCH_MIN_CHARS &&
    (searchResults.value.length || searching.value || searchError.value)
  ) {
    measureDropdown()
    dropdownOpen.value = true
  }
}

const onInteractOutside = (event) => {
  const target = event?.detail?.originalEvent?.target
  if (searchRow.value && target && searchRow.value.contains(target)) {
    // Pointer/focus landed back on the input row — keep the dropdown open.
    event.preventDefault()
    return
  }
  closeDropdown()
}

const scrollHighlightedIntoView = () => {
  nextTick(() => {
    const el = dropdownList.value?.querySelector(`[data-index="${highlightedIndex.value}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  })
}

const onArrowDown = () => {
  if (!dropdownOpen.value) {
    if (searchQuery.value.trim().length >= SEARCH_MIN_CHARS && searchResults.value.length) {
      measureDropdown()
      dropdownOpen.value = true
    }
    return
  }
  if (!searchResults.value.length) return
  highlightedIndex.value = Math.min(highlightedIndex.value + 1, searchResults.value.length - 1)
  scrollHighlightedIntoView()
}

const onArrowUp = () => {
  if (!dropdownOpen.value || !searchResults.value.length) return
  highlightedIndex.value = Math.max(highlightedIndex.value - 1, 0)
  scrollHighlightedIntoView()
}

const onEnter = () => {
  if (!dropdownOpen.value) return
  const result = searchResults.value[highlightedIndex.value]
  if (result) selectSearchResult(result)
}

const contractSummary = (result) => {
  const list = contractIdsForResult(result)
  if (list.length === 0) return ''
  if (list.length === 1) return `${t('conversation.customer.contract')} ${list[0]}`
  return t('conversation.customer.contractCount', list.length)
}

const selectSearchResult = (result) => {
  const resultContracts = contractIdsForResult(result)
  if (resultContracts.length === 0) return
  assignContract(resultContracts[0], result)
}

const assignContract = async (contractID, nextCustomer = null, nextContract = null, verificationLevelValue = 3) => {
  if (!uuid.value || !contractID || assigning.value) return
  assigning.value = true
  try {
    const resp = await api.assignCustomerToConversation(uuid.value, contractID, verificationLevelValue)
    customer.value = resp.data?.data || nextCustomer
    contract.value = (customer.value?.contracts || []).find((item) => (
      String(item.contract_id) === String(contractID)
    )) || nextContract || contract.value
    conversationStore.current.customer_id = customer.value?.customer_id || nextCustomer?.customer_id || null
    conversationStore.current.contract_id = contract.value?.contract_id || contractID
    conversationStore.current.verification_level = verificationLevelValue
    conversationStore.current.verification_message = null
    if (conversationStore.current.contact) {
      conversationStore.current.contact.linked_customer_id = conversationStore.current.customer_id
      conversationStore.current.contact.linked_customer_id_at = new Date().toISOString()
    }
    searchMode.value = false
    searchResults.value = []
    reconnectDismissed.value = false
    linkedCustomer.value = null
    closeDropdown()
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('conversation.customer.assignFailed'),
    })
  } finally {
    assigning.value = false
  }
}

const swapContract = (contractID) => {
  if (String(contractID) === String(selectedContractID.value)) return
  const next = contracts.value.find((item) => String(item.contract_id) === String(contractID))
  assignContract(next?.contract_id || contractID, customer.value, next)
}

const reconnectLinkedCustomer = () => {
  const firstContractID = firstContractIDForCustomer(linkedCustomer.value)
  if (!firstContractID) return
  assignContract(firstContractID, linkedCustomer.value, null, 1)
}

const removeCustomer = async () => {
  if (!uuid.value || assigning.value) return
  assigning.value = true
  try {
    await api.removeCustomerFromConversation(uuid.value)
    customer.value = null
    contract.value = null
    conversationStore.current.customer_id = null
    conversationStore.current.contract_id = null
    conversationStore.current.verification_level = 0
    conversationStore.current.verification_message = null
    if (conversationStore.current.contact) {
      conversationStore.current.contact.linked_customer_id = null
      conversationStore.current.contact.linked_customer_id_at = null
    }
    linkedCustomer.value = null
    searchMode.value = false
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('conversation.customer.removeSuccess'),
    })
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message,
    })
  } finally {
    assigning.value = false
  }
}

const showSearchInstead = () => {
  searchMode.value = true
  searchQuery.value = ''
  searchResults.value = []
  searchError.value = false
  closeDropdown()
}

const reIdentify = async () => {
  if (!uuid.value || running.value) return
  startingIdentification.value = true
  latestOperation.value = null
  timeoutPanel.value = false
  dismissedTraceID.value = ''
  startRunning()
  try {
    await api.identifyCustomer(uuid.value)
  } catch (error) {
    if (error?.response?.status === 409) return
    stopRunning()
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message,
    })
  } finally {
    startingIdentification.value = false
  }
}

const handleIdentificationCompleted = async (event) => {
  stopRunning()
  const previousID = stringValue(event?.previous_linked_customer_id)
  await conversationStore.fetchConversation(uuid.value)
  await refresh()
  const currentID = customer.value?.customer_id || conversationStore.current?.customer_id || ''
  if (previousID && currentID && previousID !== currentID && !isConflictDismissed(previousID, currentID)) {
    conflictNotice.value = { previousId: previousID, currentId: currentID }
    conflictDismissed.value = false
  }
  // Let showSearchState derive the final view from the freshly loaded data.
  searchMode.value = false
}

useWorkflowCompletion('CUSTOMER_IDENTIFICATION_COMPLETED', uuid, handleIdentificationCompleted)

const startRunning = () => {
  running.value = true
  searchMode.value = true
  timeoutPanel.value = false
  clearRunningTimeout()
  runningTimeout = window.setTimeout(() => {
    running.value = false
    timeoutPanel.value = true
  }, 60 * 1000)
}

const stopRunning = () => {
  running.value = false
  clearRunningTimeout()
}

const clearRunningTimeout = () => {
  if (!runningTimeout) return
  window.clearTimeout(runningTimeout)
  runningTimeout = null
}

const dismissPanel = () => {
  if (latestOperation.value?.trace_id) {
    dismissedTraceID.value = latestOperation.value.trace_id
  }
  timeoutPanel.value = false
}

const dismissReconnectPanel = () => {
  reconnectDismissed.value = true
  if (uuid.value) window.sessionStorage.setItem(reconnectStorageKey(uuid.value), '1')
}

const dismissConflictNotice = () => {
  conflictDismissed.value = true
  if (uuid.value && conflictNotice.value) {
    window.sessionStorage.setItem(
      conflictStorageKey(uuid.value, conflictNotice.value.previousId, conflictNotice.value.currentId),
      '1'
    )
  }
}

const reconnectStorageKey = (conversationUUID) => `contact-data:reconnect:${conversationUUID}`

const conflictStorageKey = (conversationUUID, previousID, currentID) => (
  `contact-data:conflict:${conversationUUID}:${previousID}:${currentID}`
)

const isConflictDismissed = (previousID, currentID) => {
  if (!uuid.value) return false
  return window.sessionStorage.getItem(conflictStorageKey(uuid.value, previousID, currentID)) === '1'
}

const loadPanelDismissals = (conversationUUID) => {
  reconnectDismissed.value = Boolean(conversationUUID) &&
    window.sessionStorage.getItem(reconnectStorageKey(conversationUUID)) === '1'
  conflictDismissed.value = false
  conflictNotice.value = null
}

const resetLocalState = () => {
  cancelInFlightSearch()
  searchSeq += 1
  customer.value = null
  contract.value = null
  linkedCustomer.value = null
  latestOperation.value = null
  searchMode.value = false
  searchQuery.value = ''
  searchResults.value = []
  searchError.value = false
  searching.value = false
  dropdownOpen.value = false
  highlightedIndex.value = -1
  assigning.value = false
  running.value = false
  timeoutPanel.value = false
  dismissedTraceID.value = ''
  reconnectDismissed.value = false
  conflictDismissed.value = false
  conflictNotice.value = null
  previewExpanded.value = false
  clearRunningTimeout()
}

watch(uuid, async (next) => {
  resetLocalState()
  loadPanelDismissals(next)
  if (next) await refresh()
}, { immediate: true })

const customerName = (row) => {
  if (!row) return '-'
  const company = stringValue(row.company)
  if (company) return company
  const first = stringValue(row.firstname)
  const last = stringValue(row.lastname)
  return [first, last].filter(Boolean).join(' ') || row.customer_id || '-'
}

function stringValue (v) {
  if (v == null) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  if (typeof v === 'object') {
    if (v.Valid === false || v.valid === false) return ''
    return v.String || v.string || v.Value || v.value || ''
  }
  return ''
}

function normalizeLinks (raw) {
  const list = normalizeArray(raw)
  return list
    .map((item) => ({
      label: item.label || item.title || item.name || '',
      url: item.url || item.href || '',
    }))
    .filter((item) => item.url)
}

function normalizePreview (raw) {
  if (!raw) return []
  if (Array.isArray(raw)) {
    return raw.map((item, index) => ({
      key: String(item.key || item.field_key || item.label || item.field_label || index),
      label: item.label || item.field_label || item.key || item.field_key || '',
      value: stringValue(item.value ?? item.display_value),
      tooltip: stringValue(item.tooltip ?? item.tool_tip),
    })).filter((item) => item.label)
  }
  if (typeof raw === 'object') {
    return Object.entries(raw).map(([key, value]) => ({
      key,
      label: key,
      value: stringValue(value),
      tooltip: '',
    }))
  }
  return []
}

function normalizeArray (raw) {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'object') return [raw]
  return []
}

function contractIdsForResult (result) {
  const raw = result?.contract_ids || []
  if (!Array.isArray(raw)) return []
  return raw.map((item) => stringValue(item)).filter(Boolean)
}

function firstContractIDForCustomer (row) {
  const contractRows = Array.isArray(row?.contracts) ? row.contracts : []
  const hydrated = contractRows.map((item) => stringValue(item?.contract_id)).find(Boolean)
  if (hydrated) return hydrated
  const rawIDs = Array.isArray(row?.contract_ids) ? row.contract_ids : []
  return rawIDs.map((item) => stringValue(item)).find(Boolean) || ''
}
</script>
