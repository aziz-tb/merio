<template>
  <div class="grid grid-cols-[minmax(0,90px)_minmax(0,1fr)] items-center gap-2 py-0.5">
    <span class="truncate text-xs text-muted-foreground">{{ label }}</span>
    <div class="flex min-w-0 justify-end">
      <slot />
    </div>
  </div>
</template>

<script setup>
defineProps({
  label: { type: String, required: true }
})
</script>
