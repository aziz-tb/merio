<!-- MessageEnvelope.vue -->
<template>
  <div class="mb-1 text-xs space-y-1">
    <div class="space-y-0.5">
      <p v-if="message.meta.from?.length">
        <span class="text-muted-foreground">From:</span>
        <template v-for="(addr, i) in message.meta.from" :key="addr">
          <router-link
            v-if="contactId && userStore.can('contacts:read') && message.type === 'incoming'"
            :to="{ name: 'contact-detail', params: { id: contactId } }"
            class="hover:underline"
          >{{ addr }}</router-link>
          <span v-else>{{ addr }}</span>
          <span v-if="i < message.meta.from.length - 1">, </span>
        </template>
      </p>
      <p v-if="message.meta.to?.length">
        <span class="text-muted-foreground">To:</span> {{ message.meta.to.join(', ') }}
      </p>
      <p v-if="message.meta.cc?.length">
        <span class="text-muted-foreground">Cc:</span> {{ message.meta.cc.join(', ') }}
      </p>
      <p v-if="message.meta.bcc?.length">
        <span class="text-muted-foreground">Bcc:</span> {{ message.meta.bcc.join(', ') }}
      </p>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useConversationStore } from '@main/stores/conversation'
import { useUserStore } from '@main/stores/user'

defineProps({
  message: {
    type: Object,
    required: true
  }
})

const conversationStore = useConversationStore()
const userStore = useUserStore()
const contactId = computed(() => conversationStore.current?.contact_id)
</script>
