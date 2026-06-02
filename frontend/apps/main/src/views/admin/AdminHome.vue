<template>
  <PageHeader />
  <div class="mx-auto flex max-w-7xl flex-col gap-10">
    <section v-for="section in visibleSections" :key="section.titleKey">
      <h2 class="mb-4 text-base font-semibold text-foreground">
        {{ t(section.titleKey, section.isTitleKeyPlural === true ? 2 : 1) }}
      </h2>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <router-link
          v-for="item in section.items"
          :key="item.href"
          :to="item.href"
          class="group flex min-h-28 items-start gap-3 rounded-lg border border-input bg-background p-4 text-left transition-colors hover:border-foreground/20 hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        >
          <div
            class="flex size-10 shrink-0 items-center justify-center rounded-lg"
            :class="section.iconBgClass"
          >
            <component :is="item.icon" class="size-5 text-foreground" />
          </div>

          <div class="min-w-0 flex-1">
            <h3 class="truncate text-sm font-semibold text-foreground">
              {{ t(item.titleKey, item.isTitleKeyPlural === true ? 2 : 1) }}
            </h3>
            <p class="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
              {{ t(item.descriptionKey) }}
            </p>
          </div>
        </router-link>
      </div>
    </section>

    <p v-if="!visibleSections.length" class="text-sm text-muted-foreground">
      {{ t('admin.empty') }}
    </p>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '@main/components/layout/PageHeader.vue'
import { adminNavItems } from '@main/constants/navigation'
import { useUserStore } from '@main/stores/user'
import { filterNavItems } from '@main/utils/nav-permissions'

const { t } = useI18n()
const userStore = useUserStore()

const visibleSections = computed(() =>
  filterNavItems(adminNavItems, userStore.can)
    .map((section) => ({
      ...section,
      items: section.children || []
    }))
    .filter((section) => section.items.length > 0)
)
</script>
