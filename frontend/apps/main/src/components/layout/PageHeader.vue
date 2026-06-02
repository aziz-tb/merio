<template>
  <div
    v-if="!isHidden"
    ref="headerRoot"
    class="sticky top-0 z-10 -mx-6 bg-background/95 px-6 backdrop-blur transition-shadow sm:-mx-8 sm:px-8"
    :class="{ 'shadow-sm': isScrolled }"
  >
    <div class="flex items-start justify-between gap-4 py-4">
      <div class="flex min-w-0 items-start gap-3">
        <button
          type="button"
          class="mt-0.5 inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
          @click="toggle"
        >
          <LayoutLeft class="size-[1.1rem]" />
          <span class="sr-only">Toggle sidebar</span>
        </button>
        <div class="min-w-0 flex-1">
          <h1 class="truncate text-xl font-semibold text-foreground">
            {{ resolvedTitle }}
          </h1>
          <p v-if="description" class="mt-1 text-sm text-muted-foreground">
            {{ description }}
          </p>
        </div>
      </div>
      <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2 pt-0.5">
        <slot name="actions" />
      </div>
    </div>
    <div v-if="$slots.tabs" class="pb-2">
      <slot name="tabs" />
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useLayoutSidebar } from '@main/composables/useLayoutSidebar'
import { LayoutLeft } from '@qall/icons'

const props = defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '' }
})

const route = useRoute()
const { t } = useI18n()
const { toggle } = useLayoutSidebar()
const headerRoot = ref(null)
const isScrolled = ref(false)

const isHidden = computed(() => route.meta?.hidePageHeader === true)

const resolvedTitle = computed(() => {
  if (props.title) return props.title
  const key = route.meta?.titleKey
  if (!key) return ''
  return t(key, route.meta?.titleCount || 1)
})

const getScrollContainer = () => headerRoot.value?.closest('.overflow-y-auto') || window

const handleScroll = () => {
  const scrollContainer = getScrollContainer()
  const scrollTop = scrollContainer === window ? window.scrollY : scrollContainer.scrollTop
  isScrolled.value = scrollTop > 4
}

onMounted(() => {
  const scrollContainer = getScrollContainer()
  scrollContainer.addEventListener('scroll', handleScroll, { passive: true })
  handleScroll()
})

onBeforeUnmount(() => {
  const scrollContainer = getScrollContainer()
  scrollContainer.removeEventListener('scroll', handleScroll)
})
</script>
