<template>
  <div class="overflow-y-auto bg-background">
    <ContactPageBanner
      :contact="contact"
      :customer="customer"
      :can-start-new-conversation="canStartNewConversation"
      @update="handleUpdate"
      @block="handleBlock"
      @unblock="handleUnblock"
      @new-conversation="handleNewConversation"
    />

    <div class="mx-auto px-6 py-6">
      <div v-if="loading && !contact" class="grid grid-cols-1 gap-4 lg:grid-cols-4">
        <div v-for="i in 4" :key="i" class="h-36 animate-pulse rounded-md bg-muted" />
      </div>

      <div v-else class="grid grid-cols-1 gap-3 lg:grid-cols-4">
        <div class="space-y-4 lg:col-span-1">
          <ContactPageCustomerCard
            :customer="customer"
            :linked-contacts="linkedContacts"
            :linked-at="contact?.linked_customer_id_at"
            :loading="customerLoading"
            @select="handleCustomerSelected"
            @unlink="handleUnlinkCustomer"
          />

          <ContactPageContractCard
            v-if="customer"
            :contract="currentContract"
            :contracts="customer.contracts || []"
            :selected-contract-id="currentContractID"
            :loading="customerLoading"
            @switch-contract="handleSwitchContract"
          />
        </div>

        <div class="lg:col-span-2">
          <ContactPageConversationHistory
            :items="conversationItems"
            :total="conversationTotal"
            :page="conversationPage"
            :page-size="conversationPageSize"
            :loading="conversationsLoading"
            @load-more="loadMoreConversations"
          />
        </div>

        <div class="lg:col-span-1">
          <Card class="p-4">
            <h3 class="mb-3 text-sm font-semibold">{{ t('globals.terms.note', 2) }}</h3>
            <ContactNotes
              v-if="contact"
              :contact-id="contact.id"
              compact
            />
          </Card>
        </div>
      </div>
    </div>

    <CreateConversation
      v-if="createConversationOpen"
      v-model="createConversationOpen"
      :initial-contact="contact"
    />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Card } from '@shared-ui/components/ui/card'
import api from '@/api'
import { EMITTER_EVENTS } from '@/constants/emitterEvents'
import { useEmitter } from '@/composables/useEmitter'
import { useAppSettingsStore } from '@/stores/appSettings'
import { useUserStore } from '@/stores/user'
import ContactNotes from '@/features/contact/ContactNotes.vue'
import ContactPageBanner from '@/features/contact/ContactPageBanner.vue'
import ContactPageContractCard from '@/features/contact/ContactPageContractCard.vue'
import ContactPageConversationHistory from '@/features/contact/ContactPageConversationHistory.vue'
import ContactPageCustomerCard from '@/features/contact/ContactPageCustomerCard.vue'
import CreateConversation from '@/features/conversation/CreateConversation.vue'

const route = useRoute()
const { t } = useI18n()
const emitter = useEmitter()
const appSettingsStore = useAppSettingsStore()
const userStore = useUserStore()

const contact = ref(null)
const customer = ref(null)
const linkedContacts = ref([])
const conversationItems = ref([])
const conversationTotal = ref(0)
const conversationPage = ref(1)
const conversationPageSize = 20
const conversationsLoading = ref(false)
const loading = ref(false)
const customerLoading = ref(false)
const createConversationOpen = ref(false)
const currentContractID = ref(null)

const contactId = computed(() => Number.parseInt(route.params.id, 10))
const canStartNewConversation = computed(() => userStore.can('conversations:write'))

const currentContract = computed(() => {
  const contracts = customer.value?.contracts || []
  if (!contracts.length) return null
  return contracts.find((contract) => contract.contract_id === currentContractID.value) || contracts[0]
})

async function fetchContact () {
  if (!contactId.value) return
  loading.value = true
  try {
    const resp = await api.getContact(contactId.value)
    contact.value = resp.data?.data || null
    if (contact.value?.linked_customer_id) {
      await fetchCustomer(stringValue(contact.value.linked_customer_id))
    } else {
      customer.value = null
      linkedContacts.value = []
      currentContractID.value = null
    }
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.loadFailed')
    })
  } finally {
    loading.value = false
  }
}

async function fetchCustomer (customerID) {
  if (!customerID) return
  customerLoading.value = true
  try {
    const resp = await api.getCustomer(customerID)
    customer.value = resp.data?.data || null
    const contracts = customer.value?.contracts || []
    if (contracts.length) {
      const stillAvailable = contracts.some((contract) => contract.contract_id === currentContractID.value)
      currentContractID.value = stillAvailable ? currentContractID.value : contracts[0].contract_id
    } else {
      currentContractID.value = null
    }
    await fetchLinkedContacts(customerID)
  } catch {
    customer.value = null
    linkedContacts.value = []
    currentContractID.value = null
  } finally {
    customerLoading.value = false
  }
}

async function fetchLinkedContacts (customerID) {
  try {
    const resp = await api.getCustomerLinkedContacts(customerID, contactId.value)
    linkedContacts.value = resp.data?.data || []
  } catch {
    linkedContacts.value = []
  }
}

async function fetchConversations (page = 1) {
  if (!contactId.value || conversationsLoading.value) return
  conversationsLoading.value = true
  try {
    const params = {
      page,
      page_size: conversationPageSize
    }
    if (route.query.exclude_uuid) params.exclude_uuid = route.query.exclude_uuid

    const resp = await api.getContactConversations(contactId.value, params)
    const data = resp.data?.data || {}
    const nextItems = data.items || []
    conversationItems.value = page === 1
      ? nextItems
      : [...conversationItems.value, ...nextItems]
    conversationTotal.value = data.total || 0
    conversationPage.value = data.page || page
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.conversationsFailed')
    })
  } finally {
    conversationsLoading.value = false
  }
}

function loadMoreConversations () {
  fetchConversations(conversationPage.value + 1)
}

async function handleUpdate (updates) {
  try {
    const resp = await api.updateContact(contactId.value, updates)
    contact.value = resp.data?.data || { ...contact.value, ...updates }
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.saveFailed')
    })
  }
}

async function handleBlock () {
  if (!window.confirm(t('contact.page.confirmBlock'))) return
  try {
    const resp = await api.blockContact(contactId.value, { enabled: false })
    contact.value = resp.data?.data || { ...contact.value, enabled: false }
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.blockFailed')
    })
  }
}

async function handleUnblock () {
  if (!window.confirm(t('contact.page.confirmUnblock'))) return
  try {
    const resp = await api.blockContact(contactId.value, { enabled: true })
    contact.value = resp.data?.data || { ...contact.value, enabled: true }
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.unblockFailed')
    })
  }
}

async function handleCustomerSelected (customerID) {
  try {
    const resp = await api.updateContactLinkedCustomer(contactId.value, customerID)
    contact.value = resp.data?.data || contact.value
    await fetchCustomer(customerID)
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      description: t('contact.page.customerLinked')
    })
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.linkCustomerFailed')
    })
  }
}

function handleNewConversation () {
  createConversationOpen.value = true
}

async function handleUnlinkCustomer () {
  if (!window.confirm(t('contact.page.confirmUnlink'))) return
  try {
    const resp = await api.updateContactLinkedCustomer(contactId.value, null)
    contact.value = resp.data?.data || contact.value
    customer.value = null
    linkedContacts.value = []
    currentContractID.value = null
  } catch {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: t('contact.page.unlinkFailed')
    })
  }
}

function handleSwitchContract (contractID) {
  currentContractID.value = contractID
}

function contactTitleName () {
  if (!contact.value) return ''
  return [contact.value.first_name, contact.value.last_name].filter(Boolean).join(' ') ||
    stringValue(contact.value.email)
}

function updateDocumentTitle () {
  const siteName = appSettingsStore.settings?.['app.site_name'] || 'libredesk'
  const name = contactTitleName()
  document.title = `${name ? `${name} · ` : ''}${t('globals.terms.contact', 2)} - ${siteName}`
}

function stringValue (value) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (typeof value === 'object') {
    if (value.Valid === false || value.valid === false) return ''
    return value.String || value.string || value.Value || value.value || ''
  }
  return ''
}

watch(contactId, async () => {
  contact.value = null
  customer.value = null
  linkedContacts.value = []
  customerLoading.value = false
  conversationItems.value = []
  conversationTotal.value = 0
  conversationPage.value = 1
  await Promise.all([
    fetchContact(),
    fetchConversations(1)
  ])
}, { immediate: true })

watch(contact, updateDocumentTitle)
</script>
