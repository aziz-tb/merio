<template>
  <div class="h-full min-h-0 p-4">
    <Tabs :default-value="defaultTab" v-model="activeTab">
      <TabsList class="mb-4 grid w-full" :class="tabsGridClass">
        <TabsTrigger v-for="(items, type) in results" :key="type" :value="type">
          {{ $t(tabLabelKeys[type], 2) }} ({{ items.length }})
        </TabsTrigger>
      </TabsList>

      <TabsContent v-for="(items, type) in results" :key="type" :value="type" class="mt-0">
        <div class="overflow-hidden rounded-md border border-border bg-background">
          <!-- No results message -->
          <div v-if="items.length === 0" class="p-8 text-center text-muted-foreground">
            <div class="text-lg font-medium mb-2">
              {{ $t('globals.messages.noResultsFound') }}
            </div>
            <div class="text-sm">{{ $t('search.adjustSearchTerms') }}</div>
          </div>

          <!-- Results list -->
          <div v-else class="divide-y divide-border">
            <div
              v-for="item in items"
              :key="item.id || item.uuid"
              class="group p-4 transition duration-200 ease-in-out hover:bg-muted/40"
            >
              <router-link
                :to="{
                  name: 'inbox-conversation',
                  params: {
                    uuid: type === 'conversations' ? item.uuid : item.conversation_uuid,
                    type: 'assigned'
                  }
                }"
                class="block"
              >
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0 flex-1">
                    <!-- Reference number and status -->
                    <div
                      class="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground transition duration-200"
                    >
                      <span>
                        #{{
                          type === 'conversations'
                            ? item.reference_number
                            : item.conversation_reference_number
                        }}
                      </span>
                      <Badge variant="outline" class="text-xs font-medium">
                        {{
                          type === 'conversations'
                            ? item.status
                            : item.conversation_status
                        }}
                      </Badge>
                    </div>

                    <!-- Content -->
                    <div
                      class="mb-2 truncate text-sm font-medium text-foreground transition duration-200"
                    >
                      {{
                        truncateText(
                          type === 'conversations' ? item.subject : item.text_content,
                          100
                        )
                      }}
                    </div>

                    <!-- Timestamp -->
                    <div class="flex items-center text-xs text-muted-foreground">
                      <ClockIcon class="mr-1 size-3.5" />
                      {{
                        formatDate(
                          type === 'conversations' ? item.created_at : item.conversation_created_at
                        )
                      }}
                    </div>
                  </div>

                  <!-- Right arrow icon -->
                  <div
                    class="rounded-full bg-secondary p-2 transition duration-200 group-hover:bg-muted"
                  >
                    <ChevronRightIcon
                      class="size-4 text-secondary-foreground"
                      aria-hidden="true"
                    />
                  </div>
                </div>
              </router-link>
            </div>
          </div>
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
<script setup>
import { computed, ref, watch } from 'vue'
import { ChevronRight as ChevronRightIcon, Clock as ClockIcon } from '@qall/icons'
import { format, parseISO } from 'date-fns'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared-ui/components/ui/tabs'
import { Badge } from '@shared-ui/components/ui/badge'

const tabLabelKeys = {
  conversations: 'globals.terms.conversation',
  messages: 'globals.terms.message'
}

const props = defineProps({
  results: {
    type: Object,
    required: true
  }
})

// Get the first available tab as default
const defaultTab = computed(() => {
  const types = Object.keys(props.results)
  return types.length > 0 ? types[0] : ''
})

const activeTab = ref('')

// Watch for changes in results and set the first tab as active
watch(
  () => props.results,
  (newResults) => {
    const types = Object.keys(newResults)
    if (types.length > 0 && !activeTab.value) {
      activeTab.value = types[0]
    }
  },
  { immediate: true }
)

// Dynamic grid class based on number of tabs
const tabsGridClass = computed(() => {
  const tabCount = Object.keys(props.results).length
  if (tabCount <= 2) return 'grid-cols-2'
  if (tabCount <= 3) return 'grid-cols-3'
  if (tabCount <= 4) return 'grid-cols-4'
  return 'grid-cols-5'
})

const formatDate = (dateString) => {
  const date = parseISO(dateString)
  return format(date, 'MMM d, yyyy HH:mm')
}

const truncateText = (text, length) => {
  if (!text) return ''
  if (text.length <= length) return text
  return text.slice(0, length) + '...'
}
</script>
