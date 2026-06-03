<!-- frontend/apps/main/src/components/datatable/DataTableRowActions.vue
  DataTableRowActions — shared dropdown menu for the per-row actions
  column of a DataTable. Replaces every dataTableDropdown.vue.

  Each item is one of:
    { label, to: RouteLocation, disabled? }       — router-link item
    { label, onClick: () => void, disabled? }     — action item
    { separator: true }                           — visual separator
    { label, destructive: true, confirm: string,  — destructive item
      onClick: () => void }                         with confirm dialog

  destructive: true styles the item in red.
  confirm:     when provided on a destructive item, clicking opens an
               AlertDialog before invoking onClick.
-->
<template>
  <DropdownMenu>
    <DropdownMenuTrigger
      class="inline-flex size-7 items-center justify-center rounded text-muted-foreground/60 opacity-0 transition-opacity hover:bg-muted hover:text-foreground data-[state=open]:opacity-100 group-hover:opacity-100"
      @click.stop
    >
      <span class="sr-only">{{ t('globals.messages.openMenu') }}</span>
      <MoreHorizontal class="w-4 h-4" />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" @click.stop>
      <template v-for="(item, idx) in items" :key="idx">
        <DropdownMenuSeparator v-if="item.separator" />
        <template v-else-if="item.to">
          <DropdownMenuItem :as-child="true" :disabled="item.disabled">
            <RouterLink :to="item.to">{{ item.label }}</RouterLink>
          </DropdownMenuItem>
        </template>
        <DropdownMenuItem
          v-else
          :disabled="item.disabled"
          :class="item.destructive ? 'text-destructive focus:text-destructive' : ''"
          @select="(e) => onItemSelect(e, item)"
        >
          {{ item.label }}
        </DropdownMenuItem>
      </template>
    </DropdownMenuContent>
  </DropdownMenu>

  <AlertDialog v-model:open="confirmOpen">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('globals.messages.areYouAbsolutelySure') }}</AlertDialogTitle>
        <AlertDialogDescription>{{ pendingItem?.confirm }}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel @click="confirmOpen = false">
          {{ t('globals.messages.cancel') }}
        </AlertDialogCancel>
        <AlertDialogAction @click="onConfirm">
          {{ pendingItem?.label || t('globals.messages.confirm') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>

<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Ellipsis as MoreHorizontal } from '@qall/icons'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@shared-ui/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared-ui/components/ui/alert-dialog'

defineProps({
  items: { type: Array, required: true },
})

const { t } = useI18n()

const confirmOpen = ref(false)
const pendingItem = ref(null)

function onItemSelect(e, item) {
  if (item.destructive && item.confirm) {
    e.preventDefault()
    pendingItem.value = item
    confirmOpen.value = true
    return
  }
  item.onClick?.()
}

function onConfirm() {
  pendingItem.value?.onClick?.()
  confirmOpen.value = false
  pendingItem.value = null
}
</script>
