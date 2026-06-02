<template>
  <PageHeader :description="description" />
  <ActivityLog />
</template>
<script setup>
import ActivityLog from '@/features/admin/activity-log/ActivityLog.vue'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { useNavDescription } from '@main/composables/useNavDescription'

const description = useNavDescription()
</script>
