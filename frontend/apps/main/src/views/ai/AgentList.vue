<template>
  <LoadingOverlay :loading="isLoading" reserve-height>
    <div class="flex flex-col h-full">
      <!-- Sub-header strip: description + primary action -->
      <div class="flex items-center justify-between px-6 pt-4 pb-3">
        <p class="text-sm text-muted-foreground">{{ $t('admin.ai.agent.help') }}</p>
        <RouterLink :to="{ name: 'agent-create-identity' }">
          <Button>{{ $t('aiAgent.new') }}</Button>
        </RouterLink>
      </div>
      <Separator />

      <!-- Filter bar -->
      <div class="flex items-center gap-2 px-6 py-3">
        <Select v-model="enabledFilter" @update:modelValue="fetchAll">
          <SelectTrigger class="w-[140px]">
            <SelectValue :placeholder="$t('globals.terms.status')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{{ $t('globals.messages.all') }}</SelectItem>
            <SelectItem value="true">{{ $t('globals.terms.enabled') }}</SelectItem>
            <SelectItem value="false">{{ $t('globals.terms.disabled') }}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <!-- Agent table -->
      <div class="flex-1 overflow-auto px-6 pb-6">
        <DataTable :columns="columns" :data="agents" :loading="isLoading" />
        <p v-if="!isLoading && agents.length === 0" class="text-center text-muted-foreground py-8">
          {{ $t('admin.ai.noAgents') }}
        </p>
      </div>
    </div>
  </LoadingOverlay>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import DataTable from '@main/components/datatable/DataTable.vue'
import { createColumns } from '@/features/ai/agents/list/dataTableColumns.js'
import { Button } from '@shared-ui/components/ui/button'
import { Separator } from '@shared-ui/components/ui/separator/index.js'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import { useEmitter } from '@/composables/useEmitter'
import { useI18n } from 'vue-i18n'
import LoadingOverlay from '@main/components/layout/LoadingOverlay.vue'
import { EMITTER_EVENTS } from '@/constants/emitterEvents.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import api from '@/api'

const agents = ref([])
const { t } = useI18n()
const isLoading = ref(false)
const emit = useEmitter()
const enabledFilter = ref('all')
const columns = createColumns(t)

onMounted(() => {
  fetchAll()
  emit.on(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

onUnmounted(() => {
  emit.off(EMITTER_EVENTS.REFRESH_LIST, refreshList)
})

const refreshList = (data) => {
  if (data?.model === 'ai_agent') fetchAll()
}

const fetchAll = async () => {
  try {
    isLoading.value = true
    const params = { page: 1, page_size: 500 }
    if (enabledFilter.value !== 'all') params.enabled = enabledFilter.value
    const resp = await api.getAIAgents(params)
    agents.value = resp.data.data?.results ?? []
  } catch (error) {
    emit.emit(EMITTER_EVENTS.SHOW_TOAST, {
      variant: 'destructive',
      description: handleHTTPError(error).message
    })
  } finally {
    isLoading.value = false
  }
}
</script>
