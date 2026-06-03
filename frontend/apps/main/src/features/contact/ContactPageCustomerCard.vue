<template>
  <Card class="group p-4">
    <div
      v-if="linkedContacts.length > 0"
      class="-mx-4 -mt-4 mb-3 rounded-t-md border-b border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground"
    >
      <Collapsible class="group/linked">
        <CollapsibleTrigger class="flex items-center gap-1 transition-colors hover:text-foreground">
          <Info class="size-3" />
          <span>{{ t('contact.page.alsoLinkedTo', { count: linkedContacts.length }) }}</span>
          <ChevronDown class="size-3 transition-transform group-data-[state=open]/linked:rotate-180" />
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-2">
          <ul class="space-y-1">
            <li v-for="linkedContact in linkedContacts" :key="linkedContact.id">
              <router-link
                :to="{ name: 'contact-detail', params: { id: linkedContact.id } }"
                class="hover:underline"
              >
                {{ contactLabel(linkedContact) }}
              </router-link>
            </li>
          </ul>
        </CollapsibleContent>
      </Collapsible>
    </div>

    <div class="mb-3 flex items-start justify-between gap-2">
      <h3 class="text-sm font-semibold">{{ t('contact.page.customer') }}</h3>
      <div
        v-if="customer && !searchMode"
        class="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100"
      >
        <Tooltip>
          <TooltipTrigger as-child>
            <Button type="button" size="icon" variant="ghost" class="size-6" @click="startSearch">
              <Search class="size-3" />
              <span class="sr-only">{{ t('contact.page.changeCustomer') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('contact.page.changeCustomer') }}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger as-child>
            <Button type="button" size="icon" variant="ghost" class="size-6" @click="$emit('unlink')">
              <Unlink class="size-3" />
              <span class="sr-only">{{ t('contact.page.unlinkCustomer') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('contact.page.unlinkCustomer') }}</TooltipContent>
        </Tooltip>
      </div>
    </div>

    <div v-if="loading" class="space-y-2">
      <div class="h-5 w-3/4 animate-pulse rounded bg-muted" />
      <div class="h-4 w-1/2 animate-pulse rounded bg-muted" />
      <div v-for="i in 4" :key="i" class="grid grid-cols-2 gap-2 pt-1">
        <div class="h-3 animate-pulse rounded bg-muted" />
        <div class="h-3 animate-pulse rounded bg-muted" />
      </div>
    </div>

    <div v-else-if="!customer || searchMode" class="space-y-3">
      <div class="flex items-start justify-between gap-2">
        <p v-if="!customer" class="text-sm text-muted-foreground">{{ t('contact.page.noCustomerLinked') }}</p>
        <Button
          v-if="customer"
          type="button"
          variant="ghost"
          size="sm"
          class="-mr-2 -mt-2 h-7 text-xs"
          @click="cancelSearch"
        >
          <X class="mr-1 size-3" />
          {{ t('globals.messages.cancel') }}
        </Button>
      </div>

      <div class="relative">
        <Search class="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          v-model="searchQuery"
          class="h-9 pl-8"
          :placeholder="t('contact.page.searchCustomerPlaceholder')"
          autocomplete="off"
          @input="debouncedSearch"
        />
      </div>

      <div v-if="searchResults.length > 0" class="max-h-60 space-y-1 overflow-y-auto">
        <button
          v-for="result in searchResults"
          :key="result.customer_id"
          type="button"
          class="w-full rounded p-2 text-left transition-colors hover:bg-muted/60"
          @click="selectCustomer(result)"
        >
          <p class="truncate text-sm font-medium">{{ customerNameFor(result) }}</p>
          <p class="truncate text-xs text-muted-foreground">{{ result.customer_id }}</p>
        </button>
      </div>

      <div v-else-if="searching" class="py-2 text-center text-sm text-muted-foreground">
        <Loader2 class="mx-auto size-4 animate-spin" />
      </div>
      <div
        v-else-if="searchQuery.trim().length >= 2"
        class="py-2 text-center text-sm text-muted-foreground"
      >
        {{ t('conversation.customer.noResults') }}
      </div>
    </div>

    <template v-else>
      <div class="mb-3 border-b border-border pb-3">
        <p class="truncate text-base font-medium">{{ customerName }}</p>
        <p class="truncate text-xs text-muted-foreground">{{ customer.customer_id }}</p>
      </div>

      <FieldRow
        v-for="key in visibleKeys"
        :key="key"
        :name="key"
        :value="customer[key]"
      />

      <Button
        v-if="orderedKeys.length > VISIBLE_FIELD_LIMIT"
        type="button"
        variant="ghost"
        size="sm"
        class="mt-2 h-7 w-full text-xs"
        @click="expanded = !expanded"
      >
        {{ expanded ? t('globals.terms.showLess') : t('globals.terms.showMore') }}
        <span>({{ t('contact.page.hiddenFieldCount', { count: orderedKeys.length - VISIBLE_FIELD_LIMIT }) }})</span>
        <ChevronUp v-if="expanded" class="ml-1 size-3" />
        <ChevronDown v-else class="ml-1 size-3" />
      </Button>
    </template>
  </Card>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDebounceFn } from '@vueuse/core'
import { ChevronDown, ChevronUp, Info, Loader as Loader2, MagnifyingGlass as Search, LinkSlash as Unlink, X } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button'
import { Card } from '@shared-ui/components/ui/card'
import { Input } from '@shared-ui/components/ui/input'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@shared-ui/components/ui/collapsible'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import api from '@/api'
import FieldRow from './FieldRow.vue'
import { SKIP_FIELDS_CUSTOMER, VISIBLE_FIELD_LIMIT, orderFields } from '@/utils/fieldRendering'

const props = defineProps({
  customer: { type: Object, default: null },
  linkedContacts: { type: Array, default: () => [] },
  linkedAt: { type: [String, Object], default: null },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['select', 'unlink'])

const { t } = useI18n()
const expanded = ref(false)
const searchMode = ref(false)
const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)

const orderedKeys = computed(() => {
  if (!props.customer) return []
  return orderFields(props.customer, SKIP_FIELDS_CUSTOMER)
})

const visibleKeys = computed(() => (
  expanded.value ? orderedKeys.value : orderedKeys.value.slice(0, VISIBLE_FIELD_LIMIT)
))

const customerName = computed(() => {
  if (!props.customer) return ''
  if (props.customer.business && props.customer.company) return props.customer.company
  return [props.customer.firstname, props.customer.lastname].filter(Boolean).join(' ') || props.customer.customer_id
})

const performSearch = async () => {
  const q = searchQuery.value.trim()
  if (q.length < 2) {
    searchResults.value = []
    return
  }
  searching.value = true
  try {
    const resp = await api.searchCustomers(q)
    searchResults.value = (resp.data?.data || []).slice(0, 20)
  } catch {
    searchResults.value = []
  } finally {
    searching.value = false
  }
}

const debouncedSearch = useDebounceFn(performSearch, 300)

watch(() => props.customer?.customer_id, () => {
  searchMode.value = false
  searchQuery.value = ''
  searchResults.value = []
  searching.value = false
})

function startSearch () {
  searchMode.value = true
  searchQuery.value = ''
  searchResults.value = []
}

function cancelSearch () {
  searchMode.value = false
  searchQuery.value = ''
  searchResults.value = []
  searching.value = false
}

function selectCustomer (customer) {
  emit('select', customer.customer_id)
  cancelSearch()
}

function customerNameFor (customer) {
  if (customer.business && customer.company) return customer.company
  return [customer.firstname, customer.lastname].filter(Boolean).join(' ') || customer.customer_id
}

function stringValue (value) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'object') return value.String || value.string || value.Value || value.value || ''
  return String(value)
}

function contactLabel (contact) {
  return stringValue(contact.email) ||
    [contact.first_name, contact.last_name].filter(Boolean).join(' ') ||
    `Contact #${contact.id}`
}

function formatDate (value) {
  const raw = stringValue(value) || value
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString()
}
</script>
