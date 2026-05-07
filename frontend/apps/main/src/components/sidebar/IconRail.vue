<!-- frontend/apps/main/src/components/sidebar/IconRail.vue
     Left icon rail with primary navigation buttons. Pure presentational; reads from userStore + last-inbox-path storage. -->
<template>
  <div class="bg-canvas fixed top-0 bottom-0 start-0 z-20 flex flex-col items-center shrink-0 py-2.5 gap-4 w-[var(--ld-sidebar-collapsed-width)]">
    <!-- Logo placeholder -->
    <div class="flex items-center justify-center"></div>

    <!-- Menu, scrollable -->
    <div class="grow w-full px-2.5 overflow-y-auto">
      <div class="flex flex-col items-center gap-1 shrink-0">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              asChild
              variant="ghost"
              size="icon"
              :data-state="route.path.startsWith('/inboxes') ? 'open' : undefined"
              class="shrink-0 rounded-md size-9 data-[state=open]:bg-primary data-[state=open]:text-primary-foreground hover:text-foreground"
            >
              <router-link :to="lastInboxPath || { name: 'inboxes' }">
                <Inbox class="!size-[18px]" />
              </router-link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right"><p>{{ t('globals.terms.inbox', 2) }}</p></TooltipContent>
        </Tooltip>

        <Tooltip v-if="userStore.can('contacts:read_all')">
          <TooltipTrigger as-child>
            <Button
              asChild
              variant="ghost"
              size="icon"
              :data-state="route.path.startsWith('/contacts') ? 'open' : undefined"
              class="shrink-0 rounded-md size-9 data-[state=open]:bg-primary data-[state=open]:text-primary-foreground hover:text-foreground"
            >
              <router-link :to="{ name: 'contacts' }">
                <BookUser class="!size-[18px]" />
              </router-link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right"><p>{{ t('globals.terms.contact', 2) }}</p></TooltipContent>
        </Tooltip>

        <Tooltip v-if="userStore.hasReportTabPermissions">
          <TooltipTrigger as-child>
            <Button
              asChild
              variant="ghost"
              size="icon"
              :data-state="route.path.startsWith('/reports') ? 'open' : undefined"
              class="shrink-0 rounded-md size-9 data-[state=open]:bg-primary data-[state=open]:text-primary-foreground hover:text-foreground"
            >
              <router-link :to="{ name: 'reports' }">
                <FileLineChart class="!size-[18px]" />
              </router-link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right"><p>{{ t('globals.terms.report', 2) }}</p></TooltipContent>
        </Tooltip>

        <Tooltip v-if="userStore.hasAdminTabPermissions">
          <TooltipTrigger as-child>
            <Button
              asChild
              variant="ghost"
              size="icon"
              :data-state="route.path.startsWith('/admin') ? 'open' : undefined"
              class="shrink-0 rounded-md size-9 data-[state=open]:bg-primary data-[state=open]:text-primary-foreground hover:text-foreground"
            >
              <router-link
                :to="{
                  name: userStore.can('general_settings:manage') ? 'general' : 'admin'
                }"
              >
                <Shield class="!size-[18px]" />
              </router-link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right"><p>{{ t('globals.terms.admin') }}</p></TooltipContent>
        </Tooltip>
      </div>
    </div>

    <!-- Footer -->
    <div class="flex flex-col items-center gap-2.5 shrink-0">
      <Tooltip>
        <TooltipTrigger as-child>
          <NotificationBell />
        </TooltipTrigger>
        <TooltipContent side="right"><p>{{ t('globals.terms.notification', 2) }}</p></TooltipContent>
      </Tooltip>
      <SidebarNavUser />
    </div>
  </div>
</template>

<script setup>
import { useStorage } from '@vueuse/core'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@main/stores/user'
import { Button } from '@shared-ui/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared-ui/components/ui/tooltip'
import { Inbox, Shield, FileLineChart, BookUser } from 'lucide-vue-next'
import NotificationBell from '@main/components/sidebar/NotificationBell.vue'
import SidebarNavUser from '@main/components/sidebar/SidebarNavUser.vue'

const route = useRoute()
const { t } = useI18n()
const userStore = useUserStore()

// Restore last inbox path so the inbox icon returns the agent to where they were.
const lastInboxPath = useStorage('lastInboxPath', '')
</script>
