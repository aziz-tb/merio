<template>
  <DropdownMenu v-model:open="open" @update:open="onOpenChange">
    <DropdownMenuTrigger
      class="flex w-fit items-center gap-1.5 rounded-lg bg-muted hover:bg-muted/70 data-[state=open]:bg-muted/70 text-sm text-foreground px-2.5 py-0.5 whitespace-nowrap transition-colors focus:outline-none focus-visible:ring-1 focus-visible:ring-ring"
    >
      <span class="truncate max-w-[200px] p-0.5">{{ fromLabel }}</span>
      <ChevronDown class="w-3.5 h-3.5 text-muted-foreground shrink-0" />
    </DropdownMenuTrigger>
    <DropdownMenuContent class="w-72" align="start">
      <template v-if="view === 'options'">
        <DropdownMenuItem @select="onPickPreviewUser">
          <span class="flex items-center gap-2">
            <Check v-if="!preview.linkedTicket.value" class="w-3.5 h-3.5 text-foreground" />
            <span v-else class="w-3.5 h-3.5" />
            <span>{{ t('admin.ai.agent.preview.fromOptions.previewUser') }}</span>
          </span>
        </DropdownMenuItem>

        <DropdownMenuItem
          disabled
          :title="t('admin.ai.agent.preview.subsystemSoon')"
          class="justify-between"
        >
          <span class="flex items-center gap-2">
            <span class="w-3.5 h-3.5" />
            <span>{{ t('admin.ai.agent.preview.fromOptions.customer') }}</span>
          </span>
          <Badge variant="secondary" class="text-[10px] uppercase">
            {{ t('globals.terms.soon') }}
          </Badge>
        </DropdownMenuItem>

        <DropdownMenuItem
          disabled
          :title="t('admin.ai.agent.preview.subsystemSoon')"
          class="justify-between"
        >
          <span class="flex items-center gap-2">
            <span class="w-3.5 h-3.5" />
            <span>{{ t('admin.ai.agent.preview.fromOptions.contract') }}</span>
          </span>
          <Badge variant="secondary" class="text-[10px] uppercase">
            {{ t('globals.terms.soon') }}
          </Badge>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem class="justify-between" @select="onOpenSearch">
          <span class="flex items-center gap-2">
            <Check v-if="preview.linkedTicket.value" class="w-3.5 h-3.5 text-foreground" />
            <span v-else class="w-3.5 h-3.5" />
            <span>{{ t('admin.ai.agent.preview.fromOptions.ticket') }}</span>
          </span>
          <ChevronRight class="w-3.5 h-3.5 text-muted-foreground" />
        </DropdownMenuItem>
      </template>

      <div v-else class="p-1 space-y-2">
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="text-muted-foreground hover:text-foreground transition-colors"
            @click="view = 'options'"
          >
            <ChevronLeft class="w-3.5 h-3.5" />
          </button>
          <Input
            v-model="query"
            type="text"
            class="flex-1"
            :placeholder="t('admin.ai.agent.realConversation.placeholder')"
            @input="onInput"
            @keydown.stop
          />
        </div>

        <div v-if="searching" class="text-xs text-muted-foreground px-1">
          {{ t('globals.messages.loading') }}
        </div>
        <div
          v-else-if="results.length === 0 && query.length >= 3"
          class="text-xs text-muted-foreground px-1 py-2"
        >
          {{ t('globals.messages.noResults') }}
        </div>
        <ul v-else-if="results.length > 0" class="max-h-64 overflow-auto">
          <DropdownMenuItem
            v-for="r in results"
            :key="r.uuid"
            class="flex-col items-start gap-0.5"
            @select="(e) => onPickResult(e, r)"
          >
            <div class="flex items-baseline gap-1.5 w-full text-xs">
              <span class="font-mono text-foreground">#{{ r.reference_number }}</span>
              <span class="truncate flex-1">{{ r.subject || t('globals.messages.noSubject') }}</span>
            </div>
            <div class="text-muted-foreground text-xs truncate">
              {{ r.status }} · {{ formatRelative(r.created_at) }}
            </div>
          </DropdownMenuItem>
        </ul>
      </div>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<script setup>
import { computed, inject, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
} from 'lucide-vue-next'
import { Input } from '@shared-ui/components/ui/input'
import { Badge } from '@shared-ui/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@shared-ui/components/ui/dropdown-menu'
import api from '@/api'
import { AGENT_PREVIEW_KEY } from '@/composables/useAgentPreview.js'

const { t } = useI18n()
const preview = inject(AGENT_PREVIEW_KEY)

const open = ref(false)
const view = ref('options') // 'options' | 'search'
const query = ref('')
const results = ref([])
const searching = ref(false)
let debounceId = null

const fromLabel = computed(() => {
  const ticket = preview.linkedTicket.value
  if (!ticket) return t('admin.ai.agent.preview.fromOptions.previewUser')
  const who = ticket.contact?.name || ticket.contact_name || ''
  return `#${ticket.reference_number || ticket.id}${who ? ' · ' + who : ''}`
})

function onOpenChange(next) {
  if (!next) view.value = 'options'
}

function onPickPreviewUser() {
  preview.reset()
}

function onOpenSearch(e) {
  e.preventDefault()
  view.value = 'search'
}

function onInput() {
  clearTimeout(debounceId)
  debounceId = setTimeout(runSearch, 250)
}

async function runSearch() {
  const q = query.value.trim()
  if (q.length < 3) {
    results.value = []
    return
  }
  searching.value = true
  try {
    const r = await api.searchConversations({ query: q })
    results.value = r.data.data || []
  } catch {
    results.value = []
  } finally {
    searching.value = false
  }
}

async function onPickResult(e, r) {
  e.preventDefault()
  try {
    const detail = await api.getConversation(r.uuid)
    const c = detail.data.data
    const ticket = {
      id: c.id,
      reference_number: c.reference_number,
      uuid: c.uuid,
      subject: c.subject,
      body: c.first_inbound_message?.text || c.last_message?.text || '',
      last_message_text: c.last_message?.text || '',
      contact_name: c.contact?.name || c.contact?.email || '',
      contact: c.contact || null,
      legitimation: c.legitimation_level ?? 0,
    }
    preview.loadFromTicket(ticket)
    open.value = false
    view.value = 'options'
    query.value = ''
    results.value = []
  } catch {
    /* keep the menu open so the operator can retry */
  }
}

function formatRelative(iso) {
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