<template>
  <div class="rounded-md border border-blue-200 bg-blue-50 p-2 text-xs text-blue-950 dark:border-blue-900/60 dark:bg-blue-950/40 dark:text-blue-100">
    <div class="flex items-start justify-between gap-2">
      <div class="flex min-w-0 gap-2">
        <CircleInfo class="mt-0.5 size-4 shrink-0" />
        <div class="min-w-0 space-y-2">
          <p class="whitespace-pre-wrap break-words">
            {{ t('conversation.contactData.reconnect.title', { name: customerName, customerId: customerID }) }}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            class="h-7 border-blue-300 bg-white/80 px-2 text-xs text-blue-950 hover:bg-blue-100 dark:border-blue-800 dark:bg-blue-950/60 dark:text-blue-100 dark:hover:bg-blue-900/60"
            @click="$emit('reconnect')"
          >
            {{ t('conversation.contactData.reconnect.button') }}
          </Button>
        </div>
      </div>
      <button
        type="button"
        class="rounded p-0.5 text-blue-800 hover:bg-blue-100 dark:text-blue-100 dark:hover:bg-blue-900/40 "
        @click="$emit('dismiss')"
      >
        <X class="size-2.5" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { CircleInfo, X } from '@qall/icons'

const props = defineProps({
  customer: { type: Object, required: true },
})

defineEmits(['reconnect', 'dismiss'])

const { t } = useI18n()

const customerID = computed(() => props.customer?.customer_id || '-')
const customerName = computed(() => {
  const company = stringValue(props.customer?.company)
  if (company) return company
  const first = stringValue(props.customer?.firstname)
  const last = stringValue(props.customer?.lastname)
  return [first, last].filter(Boolean).join(' ') || customerID.value
})

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
</script>
