<template>
  <aside
    v-if="open"
    class="fixed right-0 top-0 z-50 flex h-screen w-full max-w-[420px] shrink-0 flex-col overflow-hidden border-l border-input bg-background shadow-xl"
  >
    <div class="flex items-center justify-between gap-3 border-b border-input px-4 py-3">
      <div>
        <h3 class="text-sm font-semibold">{{ t('admin.detection.pathPicker') }}</h3>
        <p class="text-xs text-muted-foreground">{{ sourceLabel(source) }}</p>
      </div>
      <Button type="button" variant="ghost" size="icon" @click="open = false">
        <X class="size-4" />
      </Button>
    </div>

    <div class="flex-1 overflow-auto px-4 py-3">
      <div class="mt-5 space-y-4">
        <div class="space-y-2">
          <Label>{{ t('admin.detection.source') }}</Label>
          <Select v-model="source">
            <SelectTrigger>
              <SelectValue :placeholder="t('admin.detection.selectSource')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="item in availableSources" :key="item" :value="item">
                {{ sourceLabel(item) }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <template v-if="source !== 'tags'">
          <div class="space-y-2">
            <Label>{{ t('admin.detection.conversationIdentifier') }}</Label>
            <div class="flex gap-2">
              <Input
                v-model="conversationIdentifier"
                :placeholder="t('admin.detection.conversationIdentifierPlaceholder')"
                @keydown.enter.prevent="loadVariables"
              />
              <Button type="button" variant="outline" :isLoading="loading" @click="loadVariables">
                {{ t('globals.messages.load') }}
              </Button>
            </div>
          </div>

          <div v-if="loadError" class="text-sm text-destructive">{{ loadError }}</div>

          <JsonTreeViewer
            v-if="treeValue"
            :value="treeValue"
            @select-path="selectPath"
          />
        </template>

        <template v-else>
          <TagMultiSelect v-model="selectedTags" />
          <Button type="button" class="w-full" @click="selectTags">
            {{ t('admin.detection.useSelectedTags') }}
          </Button>
        </template>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import api from '@/api'
import JsonTreeViewer from './JsonTreeViewer.vue'
import TagMultiSelect from './TagMultiSelect.vue'

const open = defineModel('open', { required: true })
const props = defineProps({
  availableSources: { type: Array, default: () => ['ticket', 'contact', 'tags'] },
  initialSource: { type: String, default: 'ticket' }
})
const emit = defineEmits(['select'])
const { t } = useI18n()

const source = ref(props.initialSource)
const conversationIdentifier = ref('')
const variables = ref(null)
const loading = ref(false)
const loadError = ref('')
const selectedTags = ref([])

watch(open, (isOpen) => {
  if (isOpen) {
    source.value = props.initialSource || 'ticket'
  }
})

const treeValue = computed(() => variables.value?.[source.value] || null)

const sourceLabel = (value) => {
  const labels = {
    ticket: t('admin.detection.source.ticket'),
    contact: t('admin.detection.source.contact'),
    tags: t('admin.detection.source.tags')
  }
  return labels[value] || value
}

const loadVariables = async () => {
  const identifier = conversationIdentifier.value.trim()
  if (!identifier) return
  try {
    loading.value = true
    loadError.value = ''
    const resp = await api.getConversationVariables(identifier)
    variables.value = resp.data?.data || null
  } catch (error) {
    loadError.value = error?.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    loading.value = false
  }
}

const selectPath = (path) => {
  emit('select', { source: source.value, path })
  open.value = false
}

const selectTags = () => {
  emit('select', { source: 'tags', path: '', operator: 'in', value: selectedTags.value })
  open.value = false
}
</script>
