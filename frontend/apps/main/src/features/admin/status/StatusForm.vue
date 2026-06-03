<template>
  <form>
    <FormField v-slot="{ componentField }" name="name">
      <FormItem>
        <FormLabel>{{ $t('globals.terms.name') }}</FormLabel>
        <FormControl>
          <Input type="text" placeholder="Spam" v-bind="componentField" />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <FormField v-slot="{ componentField }" name="category">
      <FormItem class="mt-4">
        <FormLabel>{{ $t('globals.terms.category') }}</FormLabel>
        <FormControl>
          <Select v-bind="componentField">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="$t('admin.conversationStatus.category.placeholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="open">{{ $t('globals.terms.open') }}</SelectItem>
                <SelectItem value="waiting">{{ $t('globals.terms.waiting') }}</SelectItem>
                <SelectItem value="resolved">{{ $t('globals.terms.resolved') }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>

    <div class="mt-4 grid gap-4 sm:grid-cols-2">
      <FormField v-slot="{ componentField }" name="color">
        <FormItem>
          <FormLabel>{{ $t('status.color') }}</FormLabel>
          <FormControl>
            <Select v-bind="componentField">
              <SelectTrigger class="w-full">
                <SelectValue :placeholder="$t('status.selectColor')" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="color in statusColors" :key="color" :value="color">
                    <span class="flex items-center gap-2">
                      <span :class="['size-2 rounded-full', getStatusColorDotClass(color)]" />
                      <span>{{ color }}</span>
                    </span>
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>

      <FormField v-slot="{ componentField }" name="icon">
        <FormItem>
          <FormLabel>{{ $t('status.icon') }}</FormLabel>
          <FormControl>
            <Select v-bind="componentField">
              <SelectTrigger class="w-full">
                <SelectValue :placeholder="$t('status.selectIcon')" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="option in statusIconOptions" :key="option.value" :value="option.value">
                    <span class="flex items-center gap-2">
                      <component :is="STATUS_ICONS[option.icon]" class="size-4" />
                      <span>{{ option.label }}</span>
                    </span>
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormControl>
          <FormMessage />
        </FormItem>
      </FormField>
    </div>

    <slot name="footer"></slot>
  </form>
</template>

<script setup>
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@shared-ui/components/ui/form'
import { Input } from '@shared-ui/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import {
  getStatusColorDotClass,
  STATUS_COLORS,
  STATUS_ICON_OPTIONS,
  STATUS_ICONS
} from '@main/utils/statusVisuals'

const statusColors = STATUS_COLORS
const statusIconOptions = STATUS_ICON_OPTIONS
</script>
