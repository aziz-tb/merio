<template>
  <PageHeader :description="t('admin.secrets.subtitle')">
    <template #actions>
      <Button @click="openAddDialog">
        <Plus class="size-4" />
        {{ t('admin.secrets.add') }}
      </Button>
    </template>
  </PageHeader>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <DataTable
      row-id="name"
      :columns="columns"
      :data="secrets"
      :loading="isLoading"
      :searchable="false"
      :empty-text="t('admin.secrets.empty')"
    />
  </LoadingOverlay>

  <Dialog v-model:open="secretDialogOpen">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>
          {{ isEditing ? t('admin.secrets.edit') : t('admin.secrets.add') }}
        </DialogTitle>
        <DialogDescription>
          {{ isEditing ? t('admin.secrets.editDescription') : t('admin.secrets.addDescription') }}
        </DialogDescription>
      </DialogHeader>

      <form class="space-y-4" @submit.prevent="saveSecret">
        <div class="space-y-2">
          <Label for="secret-name">{{ t('globals.terms.name') }}</Label>
          <Input
            id="secret-name"
            v-model="form.name"
            :readonly="isEditing"
            :disabled="isEditing"
            autocomplete="off"
            class="font-mono"
          />
          <p v-if="nameError" class="text-sm text-destructive">{{ nameError }}</p>
        </div>

        <div class="space-y-2">
          <Label for="secret-value">{{ t('admin.secrets.value') }}</Label>
          <div class="flex gap-2">
            <Textarea
              id="secret-value"
              v-model="form.value"
              :placeholder="t('admin.secrets.valuePlaceholder')"
              :style="showValue ? {} : { WebkitTextSecurity: 'disc' }"
              class="min-h-28 font-mono"
            />
            <Button
              type="button"
              variant="outline"
              size="icon"
              :aria-label="showValue ? t('admin.secrets.hideValue') : t('admin.secrets.showValue')"
              @click="showValue = !showValue"
            >
              <EyeOff v-if="showValue" class="size-4" />
              <Eye v-else class="size-4" />
            </Button>
          </div>
          <p v-if="valueError" class="text-sm text-destructive">{{ valueError }}</p>
        </div>

        <p v-if="formError" class="text-sm text-destructive">{{ formError }}</p>

        <DialogFooter>
          <Button type="button" variant="outline" @click="secretDialogOpen = false">
            {{ t('globals.messages.cancel') }}
          </Button>
          <Button type="submit" :is-loading="isSaving">
            {{ t('globals.messages.save') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>

  <Dialog v-model:open="deleteDialogOpen">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{{ t('admin.secrets.deleteTitle') }}</DialogTitle>
        <DialogDescription>
          {{ t('admin.secrets.deleteDescription', { name: selectedSecret?.name || '' }) }}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" @click="deleteDialogOpen = false">
          {{ t('globals.messages.cancel') }}
        </Button>
        <Button variant="destructive" :is-loading="isDeleting" @click="deleteSecret">
          {{ t('globals.messages.delete') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { computed, h, onMounted, ref } from 'vue'
import { formatDistanceToNow } from 'date-fns'
import { Eye, EyeSlash as EyeOff, Plus } from '@qall/icons'
import { useI18n } from 'vue-i18n'

import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import DataTableRowActions from '@main/components/datatable/DataTableRowActions.vue'
import { actionsCol, textCol } from '@main/components/datatable/helpers'
import api from '@main/api'
import { Button } from '@shared-ui/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@shared-ui/components/ui/dialog'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Textarea } from '@shared-ui/components/ui/textarea'

const { t } = useI18n()

const namePattern = /^[a-z][a-z0-9_]{0,63}$/

const secrets = ref([])
const isLoading = ref(false)
const isSaving = ref(false)
const isDeleting = ref(false)
const secretDialogOpen = ref(false)
const deleteDialogOpen = ref(false)
const showValue = ref(false)
const selectedSecret = ref(null)
const formError = ref('')
const form = ref({ name: '', value: '' })

const isEditing = computed(() => Boolean(selectedSecret.value))
const columns = computed(() => {
  const actionColumn = actionsCol()

  return [
    textCol(t, 'name', {
      headerKey: 'globals.terms.name',
      enableSorting: false,
      enableGlobalFilter: false,
      meta: {
        label: t('globals.terms.name'),
        font: 'mono',
        width: 420
      }
    }),
    textCol(t, 'last_accessed_at', {
      headerKey: 'admin.secrets.lastUsed',
      formatter: (value) => lastUsedLabel(value),
      enableSorting: false,
      meta: {
        label: t('admin.secrets.lastUsed'),
        width: 220
      }
    }),
    {
      ...actionColumn,
      size: 72,
      meta: {
        ...actionColumn.meta,
        align: 'right',
        width: 72
      },
      cell: ({ row }) =>
        h('div', { class: 'flex justify-end' },
          h(DataTableRowActions, {
            items: rowActions(row.original)
          })
        )
    }
  ]
})
const nameError = computed(() => {
  if (!form.value.name.trim()) return t('admin.secrets.validation.nameRequired')
  if (!namePattern.test(form.value.name.trim())) return t('admin.secrets.validation.nameFormat')
  return ''
})
const valueError = computed(() => {
  if (!form.value.value) return t('admin.secrets.validation.valueRequired')
  if (new TextEncoder().encode(form.value.value).length > 5 * 1024) {
    return t('admin.secrets.validation.valueTooLarge')
  }
  return ''
})

onMounted(() => {
  fetchSecrets()
})

const fetchSecrets = async () => {
  try {
    isLoading.value = true
    const resp = await api.getSecrets()
    secrets.value = resp.data.data || []
  } finally {
    isLoading.value = false
  }
}

const openAddDialog = () => {
  selectedSecret.value = null
  form.value = { name: '', value: '' }
  formError.value = ''
  showValue.value = false
  secretDialogOpen.value = true
}

const openEditDialog = (item) => {
  selectedSecret.value = item
  form.value = { name: item.name, value: '' }
  formError.value = ''
  showValue.value = false
  secretDialogOpen.value = true
}

const openDeleteDialog = (item) => {
  selectedSecret.value = item
  deleteDialogOpen.value = true
}

const rowActions = (item) => [
  {
    label: t('globals.messages.edit'),
    onClick: () => openEditDialog(item)
  },
  {
    label: t('globals.messages.delete'),
    destructive: true,
    onClick: () => openDeleteDialog(item)
  }
]

const saveSecret = async () => {
  formError.value = ''
  if (nameError.value || valueError.value) return
  try {
    isSaving.value = true
    if (isEditing.value) {
      await api.updateSecret(form.value.name.trim(), { value: form.value.value })
    } else {
      await api.createSecret({ name: form.value.name.trim(), value: form.value.value })
    }
    secretDialogOpen.value = false
    await fetchSecrets()
  } catch (err) {
    formError.value = err.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    isSaving.value = false
  }
}

const deleteSecret = async () => {
  if (!selectedSecret.value) return
  try {
    isDeleting.value = true
    await api.deleteSecret(selectedSecret.value.name)
    deleteDialogOpen.value = false
    selectedSecret.value = null
    await fetchSecrets()
  } finally {
    isDeleting.value = false
  }
}

const lastUsedLabel = (value) => {
  if (!value) return t('admin.secrets.never')
  return formatDistanceToNow(new Date(value), { addSuffix: true })
}
</script>
