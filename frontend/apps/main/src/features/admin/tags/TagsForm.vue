<template>
  <form class="space-y-6" @submit.prevent="$emit('submit')">
    <section class="space-y-4">
      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('globals.terms.name') }}</Label>
          <Input v-model="tag.name" type="text" placeholder="billing" />
        </div>

        <div class="space-y-2">
          <Label>{{ t('admin.tags.parent') }}</Label>
          <Select v-model="parentID">
            <SelectTrigger><SelectValue :placeholder="t('globals.terms.none')" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{{ t('globals.terms.none') }}</SelectItem>
              <SelectItem v-for="item in parentOptions" :key="item.id" :value="String(item.id)">
                {{ item.name }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-3">
        <div class="space-y-2">
          <Label>{{ t('admin.tags.type') }}</Label>
          <Select v-model="tag.type">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="skill">{{ t('admin.tags.typeSkill') }}</SelectItem>
              <SelectItem value="topic">{{ t('admin.tags.typeTopic') }}</SelectItem>
              <SelectItem value="contact">{{ t('admin.tags.typeContact') }}</SelectItem>
              <SelectItem value="other">{{ t('admin.tags.typeOther') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="space-y-2">
          <Label>{{ t('admin.tags.color') }}</Label>
          <Select v-model="tag.color">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="color in colors" :key="color" :value="color">
                {{ color }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="space-y-2">
          <Label>{{ t('admin.tags.visibility') }}</Label>
          <Select v-model="tag.visibility">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="public">{{ t('admin.tags.visibilityPublic') }}</SelectItem>
              <SelectItem value="private">{{ t('admin.tags.visibilityPrivate') }}</SelectItem>
              <SelectItem value="disabled">{{ t('admin.tags.visibilityDisabled') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div class="space-y-2">
        <Label>{{ t('admin.tags.description') }}</Label>
        <Textarea v-model="tag.description" rows="3" />
      </div>
    </section>

    <section class="space-y-4 border-t border-border pt-4">
      <h3 class="text-sm font-semibold">{{ t('admin.tags.routing') }}</h3>
      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('globals.terms.sla') }}</Label>
          <Select v-model="slaPolicyID">
            <SelectTrigger><SelectValue :placeholder="t('globals.terms.none')" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{{ t('globals.terms.none') }}</SelectItem>
              <SelectItem v-for="sla in slas" :key="sla.id" :value="String(sla.id)">
                {{ sla.name }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="space-y-2">
          <Label>{{ t('globals.terms.priority') }}</Label>
          <Select v-model="priorityID">
            <SelectTrigger><SelectValue :placeholder="t('globals.terms.none')" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{{ t('globals.terms.none') }}</SelectItem>
              <SelectItem v-for="priority in priorities" :key="priority.id" :value="String(priority.id)">
                {{ priority.name }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </section>

    <section class="space-y-4 border-t border-border pt-4">
      <h3 class="text-sm font-semibold">{{ t('admin.tags.detection') }}</h3>
      <DetectionBuilder v-model="tag.detection" :available-sources="['ticket', 'customer', 'contract', 'tags']" />
    </section>

    <TagTestRunner v-if="isEditing && tag.id" :tag-id="tag.id" />
  </form>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label } from '@shared-ui/components/ui/label'
import { Input } from '@shared-ui/components/ui/input'
import { Textarea } from '@shared-ui/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import { TAG_COLORS } from '@shared-ui/utils/tag'
import DetectionBuilder from '@/features/admin/detection/DetectionBuilder.vue'
import TagTestRunner from './TagTestRunner.vue'

const tag = defineModel({ required: true })
defineEmits(['submit'])

const props = defineProps({
  tags: { type: Array, default: () => [] },
  slas: { type: Array, default: () => [] },
  priorities: { type: Array, default: () => [] },
  isEditing: { type: Boolean, default: false }
})

const { t } = useI18n()
const colors = TAG_COLORS

const idValue = (value) => {
  if (value && typeof value === 'object') return value.Valid ? value.Int : null
  return value || null
}

const parentOptions = computed(() => {
  const currentID = tag.value.id
  return props.tags.filter((item) => {
    if (item.id === currentID) return false
    if (idValue(item.parent_id)) return false
    if (idValue(item.parent_id) === currentID) return false
    return true
  })
})

const toSelect = (value) => {
  const id = idValue(value)
  return id ? String(id) : 'none'
}

const fromSelect = (value) => {
  if (!value || value === 'none') return null
  return Number(value)
}

const parentID = computed({
  get: () => toSelect(tag.value.parent_id),
  set: (value) => { tag.value.parent_id = fromSelect(value) }
})

const slaPolicyID = computed({
  get: () => toSelect(tag.value.sla_policy_id),
  set: (value) => { tag.value.sla_policy_id = fromSelect(value) }
})

const priorityID = computed({
  get: () => toSelect(tag.value.priority_id),
  set: (value) => { tag.value.priority_id = fromSelect(value) }
})
</script>
