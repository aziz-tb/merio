<template>
  <div class="border-b border-border bg-card pb-4">
    <div class="container mx-auto w-full px-6 py-4">
      <div class="flex items-start gap-4">
        <Avatar class="size-12 shrink-0">
          <AvatarImage v-if="avatarURL" :src="avatarURL" :alt="displayName" />
          <AvatarFallback class="bg-muted">
            <User class="size-6 text-muted-foreground" />
          </AvatarFallback>
        </Avatar>

        <div class="min-w-0 flex-1">
          <div class="flex min-w-0 items-center gap-2 group/name">
            <EditableField
              :model-value="contactName"
              :placeholder="t('contact.page.namePlaceholder')"
              variant="heading"
              @commit="updateContactName"
            />
            <Pen class="size-4 text-muted-foreground opacity-0 transition-opacity group-hover/name:opacity-50" />
          </div>

          <p class="mt-0.5 truncate text-sm text-muted-foreground">{{ stringValue(contact?.email) }}</p>

          <div
            v-if="customer"
            class="mt-2 inline-flex max-w-full items-center gap-1.5 text-xs text-muted-foreground"
          >
            <LinkSimple class="size-3 shrink-0" />
            <span class="truncate">{{ customerName }}</span>
            <span>·</span>
            <span class="shrink-0 font-mono">{{ customer.customer_id }}</span>
            <template v-if="contact?.linked_customer_id_at">
              <span>·</span>
              <span class="shrink-0">{{ t('contact.page.linkedOn', { date: formatDate(contact.linked_customer_id_at) }) }}</span>
            </template>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <Button
            v-if="canStartNewConversation"
            type="button"
            size="sm"
            @click="$emit('newConversation')"
          >
            <Plus class="mr-1 size-4" />
            {{ t('contact.page.newConversation') }}
          </Button>

          <DropdownMenu v-if="contact">
            <DropdownMenuTrigger as-child>
              <Button type="button" variant="ghost" size="icon" class="size-8">
                <EllipsisVertical class="size-4" />
                <span class="sr-only">{{ t('globals.terms.openMenu') }}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem v-if="!contact.enabled" @click="$emit('unblock')">
                {{ t('contact.page.unblock') }}
              </DropdownMenuItem>
              <DropdownMenuItem v-else class="text-destructive" @click="$emit('block')">
                {{ t('contact.page.block') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, EllipsisVertical, LinkSimple, Pen, User } from '@qall/icons'
import { Avatar, AvatarFallback, AvatarImage } from '@shared-ui/components/ui/avatar'
import { Button } from '@shared-ui/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@shared-ui/components/ui/dropdown-menu'
import EditableField from './EditableField.vue'

const props = defineProps({
  contact: { type: Object, default: null },
  customer: { type: Object, default: null },
  canStartNewConversation: { type: Boolean, default: false }
})

const emit = defineEmits(['update', 'block', 'unblock', 'newConversation'])

const { t } = useI18n()

const avatarURL = computed(() => stringValue(props.contact?.avatar_url))

const contactName = computed(() => [
  stringValue(props.contact?.first_name),
  stringValue(props.contact?.last_name)
].filter(Boolean).join(' '))

const displayName = computed(() => contactName.value || stringValue(props.contact?.email) || t('globals.terms.contact'))

const customerName = computed(() => {
  if (!props.customer) return ''
  if (props.customer.business && props.customer.company) return props.customer.company
  return [props.customer.firstname, props.customer.lastname].filter(Boolean).join(' ') || props.customer.customer_id
})

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

function formatDate (value) {
  const raw = stringValue(value) || value
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString()
}

function updateContactName (value) {
  const normalized = String(value || '').trim().replace(/\s+/g, ' ')
  const [firstName = '', ...rest] = normalized.split(' ')
  emit('update', {
    first_name: firstName,
    last_name: rest.join(' ')
  })
}
</script>
