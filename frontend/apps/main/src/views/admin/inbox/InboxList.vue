<template>
  <PageHeader :description="description">
    <template #actions>
      <router-link :to="{ name: 'new-inbox' }">
        <Button>
          {{
            $t('inbox.newInbox')
          }}
        </Button>
      </router-link>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable :columns="columns" :data="data" :loading="isLoading" :searchable="true" />
  </LoadingOverlay>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { h } from 'vue'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { Button } from '@shared-ui/components/ui/button'
import DataTable from '@main/components/datatable/DataTable.vue'
import DataTableRowActions from '@main/components/datatable/DataTableRowActions.vue'
import { actionsCol, dateTimeCol, linkCol } from '@main/components/datatable/helpers'
import { EMITTER_EVENTS } from '@main/constants/emitterEvents.js'
import { useEmitter } from '@main/composables/useEmitter'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'
import { useInboxStore } from '@main/stores/inbox'
import api from '@main/api'

const { t } = useI18n()
const route = useRoute()
const emitter = useEmitter()
const inboxStore = useInboxStore()
const isLoading = ref(false)
const data = ref([])
const description = useNavDescription()
const actionColumn = actionsCol()

onMounted(async () => {
  // Handle OAuth callback messages
  const errorCode = route.query.error
  const successCode = route.query.success

  if (errorCode) {
    let msg
    if (errorCode === 'oauth_denied') {
      msg = t('toast.authorizationDenied')
    } else if (errorCode === 'inbox_already_exists') {
      msg = t('inbox.oauthAlreadyExists')
    } else if (errorCode === 'inbox_not_found') {
      msg = t('inbox.oauthNotFound')
    } else if (errorCode === 'email_mismatch') {
      msg = t('inbox.oauthEmailMismatch')
    } else {
      msg = t('toast.errorConnectingInbox')
    }
    setTimeout(() => {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
        variant: 'destructive',
        description: msg
      })
    }, 500)
  } else if (successCode) {
    const msg =
      successCode === 'oauth_reconnected'
        ? t('toast.inboxReconnected')
        : t('toast.inboxConnected')
    setTimeout(() => {
      emitter.emit(EMITTER_EVENTS.SHOW_TOAST, { description: msg })
    }, 500)
  }

  await getInboxes()
})

const getInboxes = async () => {
  try {
    isLoading.value = true
    await inboxStore.fetchInboxes(true)
    data.value = inboxStore.inboxes
  } catch (error) {
    emitter.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}

// Columns for the data table
const columns = [
  linkCol(t, 'name', {
    to: (row) => ({ name: 'edit-inbox', params: { id: row.id } })
  }),
  {
    accessorKey: 'channel',
    header: () => t('globals.terms.channel'),
    cell: ({ row }) => row.getValue('channel')
  },
  {
    accessorKey: 'enabled',
    header: () => t('globals.terms.enabled'),
    cell: ({ row }) => {
      const enabled = row.getValue('enabled')
      return enabled ? 'Yes' : 'No'
    }
  },
  dateTimeCol(t, 'created_at'),
  dateTimeCol(t, 'updated_at'),
  {
    ...actionColumn,
    size: 72,
    meta: {
      ...actionColumn.meta,
      align: 'right',
      width: 72
    },
    cell: ({ row }) => {
      const inbox = row.original
      return h('div', { class: 'flex justify-end' },
        h(DataTableRowActions, {
          items: [
            {
              label: t('globals.messages.edit'),
              to: { name: 'edit-inbox', params: { id: inbox.id } }
            },
            {
              label: t('globals.messages.delete'),
              destructive: true,
              confirm: t('confirm.deleteInbox'),
              onClick: () => handleDeleteInbox(inbox.id)
            },
            {
              label: inbox.enabled ? t('globals.messages.disable') : t('globals.messages.enable'),
              onClick: () => handleToggleInbox(inbox.id)
            }
          ]
        })
      )
    }
  }
]

const handleDeleteInbox = async (id) => {
  await api.deleteInbox(id)
  getInboxes()
}

const handleToggleInbox = async (id) => {
  await api.toggleInbox(id)
  getInboxes()
}
</script>
