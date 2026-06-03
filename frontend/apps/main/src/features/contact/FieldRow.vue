<template>
  <div class="grid grid-cols-2 justify-around gap-2 py-1 text-xs">
    <span class="break-words text-muted-foreground">{{ humanizeKey(name) }}</span>
    <div class="min-w-0">
      <span v-if="formatted.kind === 'simple'" class="break-words text-foreground">
        {{ formatted.display }}
      </span>

      <Collapsible v-else class="group/field">
        <CollapsibleTrigger class="flex items-center gap-1 text-left text-foreground transition-colors hover:text-primary">
          <span>{{ formatted.display }}</span>
          <ChevronDown class="size-3 transition-transform group-data-[state=open]/field:rotate-180" />
        </CollapsibleTrigger>
        <CollapsibleContent class="mt-1 border-l border-muted pl-2">
          <pre class="whitespace-pre-wrap break-all text-xs text-muted-foreground">{{ JSON.stringify(formatted.raw, null, 2) }}</pre>
        </CollapsibleContent>
      </Collapsible>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ChevronDown } from '@qall/icons'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@shared-ui/components/ui/collapsible'
import { formatValue, humanizeKey } from '@/utils/fieldRendering'

const props = defineProps({
  name: { type: String, required: true },
  value: { type: null, default: null }
})

const formatted = computed(() => formatValue(props.value))
</script>
