<template>
  <router-view v-if="isFormRoute" />
  <div v-else class="h-full overflow-y-auto">
    <div class="px-6 py-6 sm:px-8">
      <router-view />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

defineOptions({ name: 'KnowledgePage' })

const route = useRoute()
const isFormRoute = computed(() =>
  ['new-article', 'edit-article'].includes(route.name)
)
</script>
