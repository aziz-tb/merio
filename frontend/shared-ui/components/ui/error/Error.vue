<script setup lang="ts">
import { TriangleExclamation } from '@qall/icons'
import { Alert, AlertDescription, AlertTitle } from '../alert'

defineProps({
  errorMessage: String,
  border: Boolean
})
</script>

<template>
  <Alert variant="destructive" v-if="errorMessage" :class="{ 'border-0': !border }">
    <TriangleExclamation class="w-4 h-4" />
    <AlertTitle>Error</AlertTitle>
    <AlertDescription>
      {{ errorMessage }}
    </AlertDescription>
  </Alert>
</template>
