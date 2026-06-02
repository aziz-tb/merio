<template>
  <Card class="p-4">
    <div v-if="loading" class="space-y-2">
      <div class="h-5 w-2/3 animate-pulse rounded bg-muted" />
      <div class="h-4 w-1/2 animate-pulse rounded bg-muted" />
      <div v-for="i in 4" :key="i" class="grid grid-cols-2 gap-2 pt-1">
        <div class="h-3 animate-pulse rounded bg-muted" />
        <div class="h-3 animate-pulse rounded bg-muted" />
      </div>
    </div>

    <div v-else-if="!contract" class="py-4 text-center">
      <p class="text-xs text-muted-foreground">{{ t('contact.page.noContract') }}</p>
    </div>

    <template v-else>
      <div class="mb-3 space-y-2 border-b border-border pb-3">
        <h3 class="text-sm font-semibold">{{ t('contact.page.contract') }}</h3>

        <PillSwitcher
          v-if="contracts.length > 1"
          :model-value="selectedContractId"
          @update:modelValue="$emit('switchContract', $event)"
        >
          <PillSwitcherItem
            v-for="item in contracts"
            :key="item.contract_id"
            :value="item.contract_id"
            class="min-w-0 text-xs"
          >
            <span class="truncate">{{ item.contract_id }}</span>
          </PillSwitcherItem>
        </PillSwitcher>

        <p v-else class="truncate text-sm font-medium">{{ contract.contract_id }}</p>

        <div v-if="erpLinks.length" class="flex flex-wrap gap-1.5">
          <Button
            v-for="link in erpLinks"
            :key="link.url"
            as="a"
            variant="outline"
            size="sm"
            class="h-7 px-2 text-xs w-full"
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
          >
            <ExternalLink class="mr-1 size-3" />
            {{ link.label || t('conversation.customer.openInERP') }}
          </Button>
        </div>
      </div>

      <FieldRow
        v-for="key in visibleKeys"
        :key="key"
        :name="key"
        :value="contract[key]"
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
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronUp, ExternalLink } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Card } from '@shared-ui/components/ui/card'
import { PillSwitcher, PillSwitcherItem } from '@shared-ui/components/ui/pill-switcher'
import FieldRow from './FieldRow.vue'
import { SKIP_FIELDS_CONTRACT, VISIBLE_FIELD_LIMIT, orderFields } from '@/utils/fieldRendering'

const props = defineProps({
  contract: { type: Object, default: null },
  contracts: { type: Array, default: () => [] },
  selectedContractId: { type: [String, Number], default: null },
  loading: { type: Boolean, default: false }
})

defineEmits(['switchContract'])

const { t } = useI18n()
const expanded = ref(false)

const orderedKeys = computed(() => {
  if (!props.contract) return []
  return orderFields(props.contract, SKIP_FIELDS_CONTRACT)
})

const visibleKeys = computed(() => (
  expanded.value ? orderedKeys.value : orderedKeys.value.slice(0, VISIBLE_FIELD_LIMIT)
))

const erpLinks = computed(() => {
  const raw = props.contract?.erp_urls
  if (!Array.isArray(raw)) return []
  return raw
    .map((link) => ({
      label: link?.label || link?.title || link?.name || '',
      url: link?.url || link?.href || ''
    }))
    .filter((link) => link.url)
})
</script>
