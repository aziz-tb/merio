<template>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <div class="flex h-full flex-col gap-4 bg-background p-6">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold tracking-normal">{{ t('admin.secrets.title') }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.secrets.subtitle') }}</p>
        </div>
        <Button @click="openAddDialog">
          <Plus class="mr-2 size-4" />
          {{ t('admin.secrets.add') }}
        </Button>
      </div>

      <div class="overflow-hidden rounded-md border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-7/12">{{ t('globals.terms.name') }}</TableHead>
              <TableHead>{{ t('admin.secrets.lastUsed') }}</TableHead>
              <TableHead class="w-16 text-right">{{ t('globals.terms.action', 2) }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-if="secrets.length === 0">
              <TableCell colspan="3" class="h-28 text-center text-sm text-muted-foreground">
                {{ t('admin.secrets.empty') }}
              </TableCell>
            </TableRow>
            <TableRow v-for="item in secrets" :key="item.name">
              <TableCell>
                <div class="font-mono text-sm">{{ item.name }}</div>
              </TableCell>
              <TableCell class="text-sm text-muted-foreground">
                {{ lastUsedLabel(item.last_accessed_at) }}
              </TableCell>
              <TableCell class="text-right">
                <DropdownMenu>
                  <DropdownMenuTrigger as-child>
                    <Button variant="ghost" size="icon" :aria-label="t('globals.messages.openMenu')">
                      <MoreHorizontal class="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem @click="openEditDialog(item)">
                      <Pencil class="mr-2 size-4" />
                      {{ t('globals.messages.edit') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem class="text-destructive" @click="openDeleteDialog(item)">
                      <Trash2 class="mr-2 size-4" />
                      {{ t('globals.messages.delete') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>
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
import { computed, onMounted, ref } from 'vue'
import { formatDistanceToNow } from 'date-fns'
import { Eye, EyeOff, MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@shared-ui/components/ui/table'
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
