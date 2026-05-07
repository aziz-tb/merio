<template>
  <div v-if="!isHidden">
    <div class="flex items-center space-x-4 h-12 px-2">
      <button
        type="button"
        class="cursor-pointer inline-flex items-center justify-center size-8 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground"
        @click="toggle"
      >
        <PanelLeft class="size-4" />
        <span class="sr-only">Toggle sidebar</span>
      </button>
      <span class="text-xl font-semibold">
        {{ title }}
      </span>
    </div>
    <Separator />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Separator } from '@shared-ui/components/ui/separator'
import { PanelLeft } from 'lucide-vue-next'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'

const route = useRoute()
const { t } = useI18n()
const { toggle } = useLayoutSidebar()
const title = computed(() => {
  const key = route.meta?.titleKey
  if (!key) return ''
  return t(key, route.meta?.titleCount || 1)
})
const isHidden = computed(() => route.meta.hidePageHeader === true)
</script>
