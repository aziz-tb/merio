<template>
  <PageHeader :description="description">
    <template #actions>
      <Button @click="newRule">{{
        $t('automation.newRule')
      }}</Button>
    </template>
  </PageHeader>
  <div v-if="selectedTab">
    <AutomationTabs v-model:automationsTab="selectedTab" />
  </div>
</template>

<script setup>
import { Button } from '@shared-ui/components/ui/button'
import { useRouter } from 'vue-router'
import { useStorage } from '@vueuse/core'
import AutomationTabs from '@/features/admin/automation/AutomationTabs.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'

const router = useRouter()
const selectedTab = useStorage('automationsTab', 'new_conversation')
const description = useNavDescription()
const newRule = () => {
  router.push({ name: 'new-automation', query: { type: selectedTab.value } })
}
</script>
