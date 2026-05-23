<template>
  <div class="space-y-2">
    <Label class="text-muted-foreground">
      {{ $t('admin.ai.agent.realConversation.label') }}
    </Label>
    <Input
      v-model="query"
      :placeholder="$t('admin.ai.agent.realConversation.placeholder')"
      @input="onInput"
    />
    <div v-if="loading" class="text-xs text-muted-foreground">
      {{ $t('globals.messages.loading') }}
    </div>
    <ul v-if="results.length > 0" class="border rounded-md max-h-56 overflow-y-auto divide-y">
      <li
        v-for="r in results"
        :key="r.uuid"
        class="px-2 py-1.5 text-xs hover:bg-muted cursor-pointer"
        @click="pick(r)"
      >
        <span class="font-mono">#{{ r.reference_number }}</span>
        ·
        <span>{{ r.subject || $t('globals.messages.noSubject') }}</span>
        ·
        <span class="text-muted-foreground">{{ r.status }}</span>
        ·
        <span class="text-muted-foreground">{{ formatRelative(r.created_at) }}</span>
      </li>
    </ul>
    <div v-if="picked" class="flex items-center gap-2 text-xs">
      <Badge variant="secondary">
        {{ $t('admin.ai.agent.realConversation.selected') }}
      </Badge>
      <span class="font-mono">#{{ picked.reference_number }}</span>
      <span>· {{ picked.subject }}</span>
      <button type="button" class="text-muted-foreground underline" @click="clear">
        {{ $t('globals.messages.clear') }}
      </button>
    </div>
  </div>
</template>

<script setup>
// RealConversationSelect — typeahead picker for the agent test panel.
// Drives the runner's conversation_id field, so an admin can replay
// against a real ticket. The /api/v1/conversations/search endpoint
// returns UUID + subject + reference number; we fetch by UUID on
// selection to resolve the numeric id the runner expects.
import { ref } from 'vue'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'
import { Badge } from '@shared-ui/components/ui/badge'
import api from '@/api'

const props = defineProps({
  modelValue: { type: [Number, null], default: null }
})
const emit = defineEmits(['update:modelValue'])

const query = ref('')
const results = ref([])
const loading = ref(false)
const picked = ref(null)
let debounce = null

const onInput = () => {
  clearTimeout(debounce)
  debounce = setTimeout(runSearch, 250)
}

const runSearch = async () => {
  const q = query.value.trim()
  if (q.length < 3) {
    results.value = []
    return
  }
  loading.value = true
  try {
    const r = await api.searchConversations({ query: q })
    results.value = r.data.data || []
  } catch {
    results.value = []
  } finally {
    loading.value = false
  }
}

const pick = async (r) => {
  try {
    const detail = await api.getConversation(r.uuid)
    const c = detail.data.data
    picked.value = r
    results.value = []
    query.value = ''
    emit('update:modelValue', c.id)
  } catch {
    /* swallow — admin can try again */
  }
}

const clear = () => {
  picked.value = null
  emit('update:modelValue', null)
}

// Lightweight relative-time formatter for the result row. Tooling
// (luxon, dayjs) isn't pulled in just for this picker.
const formatRelative = (iso) => {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diff = Date.now() - then
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}
</script>
