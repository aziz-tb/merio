<template>
  <div class="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-sidebar px-3">
    <LayoutSidebarTrigger
      class="cursor-pointer inline-flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
    >
      <LayoutLeft class="size-4" />
    </LayoutSidebarTrigger>
    <div class="relative w-full flex-1">
      <MagnifyingGlass class="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        v-model="model"
        :placeholder="t('globals.terms.search')"
        class="h-8 border-border/60 bg-transparent pl-9 text-xs shadow-none focus-visible:bg-background"
      />
    </div>
  </div>
</template>

<script setup>
import { Input } from '@shared-ui/components/ui/input'
import LayoutSidebarTrigger from '@main/components/sidebar/LayoutSidebarTrigger.vue'
import { useI18n } from 'vue-i18n'
import { LayoutLeft, MagnifyingGlass } from '@qall/icons'
const model = defineModel({
  type: String,
  default: '',
  required: false
})
const { t } = useI18n()
</script>
