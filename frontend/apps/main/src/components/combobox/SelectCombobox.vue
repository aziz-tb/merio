<template>
  <ComboBox
    :model-value="normalizedValue"
    @update:model-value="$emit('update:modelValue', $event)"
    @select="$emit('select', $event)"
    :items="items"
    :placeholder="placeholder"
    :trigger-variant="triggerVariant"
    :button-class="buttonClass"
  >
    <!-- Items -->
    <template #item="{ item }">
      <div class="flex items-center gap-2">
        <!-- USER -->
        <Avatar v-if="type === 'user'" class="w-7 h-7">
          <template v-if="!isNoneItem(item)">
            <AvatarImage :src="item.avatar_url || ''" :alt="item.label.slice(0, 2)" />
            <AvatarFallback>{{ item.label.slice(0, 2).toUpperCase() }}</AvatarFallback>
          </template>
          <AvatarFallback v-else class="bg-muted">
            <User class="w-4 h-4 text-muted-foreground" />
          </AvatarFallback>
        </Avatar>

        <!-- TEAM -->
        <Avatar v-else-if="type === 'team'" class="w-7 h-7">
          <AvatarFallback v-if="!isNoneItem(item)">
            {{ item.label.slice(0, 2).toUpperCase() }}
          </AvatarFallback>
          <AvatarFallback v-else class="bg-muted">
            <Users class="w-4 h-4 text-muted-foreground" />
          </AvatarFallback>
        </Avatar>

        <!-- Others -->
        <component
          v-else-if="item.icon && PRIORITY_ICONS[item.icon]"
          :is="PRIORITY_ICONS[item.icon]"
          class="size-4 shrink-0"
        />
        <span v-else-if="item.emoji">{{ item.emoji }}</span>
        <span>{{ item.label }}</span>
        <span v-if="isCurrentUser(item)" class="text-muted-foreground text-xs"
          >({{ t('globals.terms.you') }})</span
        >
      </div>
    </template>

    <!-- Selected -->
    <template #selected="{ selected }">
      <div class="flex items-center gap-2 min-w-0">
        <div
          v-if="selected && type === 'user' && !isNoneItem(selected)"
          class="flex items-center gap-2 min-w-0"
        >
          <Avatar :class="[selectedAvatarClass, 'shrink-0']">
            <AvatarImage :src="selected.avatar_url || ''" :alt="selected.label.slice(0, 2)" />
            <AvatarFallback :class="selectedFallbackClass">
              {{ selected.label.slice(0, 2).toUpperCase() }}
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-sm">{{ selected.label }}</span>
          <span
            v-if="isCurrentUser(selected)"
            class="text-muted-foreground text-xs shrink-0"
            >({{ t('globals.terms.you') }})</span
          >
        </div>

        <div
          v-else-if="selected && type === 'team' && !isNoneItem(selected)"
          class="flex items-center gap-2 min-w-0"
        >
          <Avatar :class="[selectedAvatarClass, 'shrink-0']">
            <AvatarFallback :class="selectedFallbackClass">
              {{ selected.label.slice(0, 2).toUpperCase() }}
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-sm">{{ selected.label }}</span>
        </div>

        <div
          v-else-if="selected && selected.icon && PRIORITY_ICONS[selected.icon]"
          class="flex items-center gap-2 min-w-0"
        >
          <component
            :is="PRIORITY_ICONS[selected.icon]"
            class="size-4 shrink-0"
          />
          <span class="truncate text-sm">{{ selected.label }}</span>
        </div>

        <div
          v-else-if="selected && selected.emoji"
          class="flex items-center gap-2 min-w-0"
        >
          <span class="shrink-0">{{ selected.emoji }}</span>
          <span class="truncate text-sm">{{ selected.label }}</span>
        </div>

        <div
          v-else-if="selected"
          class="flex items-center gap-2 min-w-0"
        >
          <span class="truncate text-sm">{{ selected.label }}</span>
        </div>

        <div v-else-if="type === 'user'" class="flex items-center gap-2 min-w-0 text-muted-foreground">
          <Avatar :class="[selectedAvatarClass, 'shrink-0']">
            <AvatarFallback class="bg-muted">
              <User class="w-2.5 h-2.5 text-muted-foreground" />
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-sm">{{ placeholder }}</span>
        </div>

        <div v-else-if="type === 'team'" class="flex items-center gap-2 min-w-0 text-muted-foreground">
          <Avatar :class="[selectedAvatarClass, 'shrink-0']">
            <AvatarFallback class="bg-muted">
              <Users class="w-3 h-3 text-muted-foreground" />
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-sm">{{ placeholder }}</span>
        </div>

        <div v-else-if="type === 'priority'" class="flex items-center gap-2 min-w-0">
          <component
            v-if="priorityPlaceholderIcon"
            :is="priorityPlaceholderIcon"
            class="size-4 shrink-0"
          />
          <span class="truncate text-sm text-muted-foreground">{{ placeholder }}</span>
        </div>

        <div v-else class="flex items-center gap-2 min-w-0 text-muted-foreground">
          <span class="truncate text-sm">{{ placeholder }}</span>
        </div>
      </div>
    </template>
  </ComboBox>
</template>

<script setup>
import { computed } from 'vue'
import { Avatar, AvatarImage, AvatarFallback } from '@shared-ui/components/ui/avatar'
import ComboBox from '@shared-ui/components/ui/combobox/ComboBox.vue'
import { useUserStore } from '@/stores/user'
import { useI18n } from 'vue-i18n'
import { User, Users } from '@qall/icons'
import { FALLBACK_PRIORITY_VISUAL, PRIORITY_ICONS } from '@/utils/priorityVisuals'
const { t } = useI18n()
const userStore = useUserStore()

const props = defineProps({
  modelValue: [String, Number, Object],
  placeholder: String,
  items: Array,
  type: {
    type: String
  },
  triggerVariant: {
    type: String,
    default: 'pill'
  },
  buttonClass: {
    type: String,
    default: ''
  }
})

// Convert to str.
const normalizedValue = computed(() => String(props.modelValue || ''))
const isInlineTrigger = computed(() => props.triggerVariant === 'inline')
const selectedAvatarClass = computed(() => isInlineTrigger.value ? 'w-5 h-5' : 'w-7 h-7')
const selectedFallbackClass = computed(() => isInlineTrigger.value ? 'text-[10px]' : '')
const priorityPlaceholderIcon = computed(() => PRIORITY_ICONS[FALLBACK_PRIORITY_VISUAL.icon])

const isCurrentUser = (item) => {
  return (
    props.type === 'user' &&
    !isNoneItem(item) &&
    String(item.value) === String(userStore.userID)
  )
}

const isNoneItem = (item) => String(item?.value) === 'none'

defineEmits(['update:modelValue', 'select'])
</script>
