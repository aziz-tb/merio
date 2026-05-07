<template>
  <div class="flex items-center gap-3 px-3 py-3 border-b min-w-[200px]">
    <Skeleton class="h-10 w-10 rounded-full aspect-square" />
    <div class="space-y-1 flex-grow">
      <Skeleton class="h-3.5 w-full max-w-[250px]" />
      <Skeleton class="h-3 w-full max-w-[180px]" />
      <Skeleton class="h-3 w-full max-w-[140px]" />
    </div>
  </div>
</template>

<script setup>
import { Skeleton } from '@shared-ui/components/ui/skeleton'
</script>
