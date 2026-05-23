<template>
  <div class="grid gap-2 rounded border border-border p-3 md:grid-cols-[1fr_11rem_1fr_auto]">
    <div class="relative">
      <Input
        v-model="displayPathValue"
        readonly
        class="pr-9"
        :placeholder="t('admin.detection.path')"
        @click="openPicker"
      />
      <button
        type="button"
        class="absolute right-2 top-1/2 size-5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
        :title="t('admin.detection.pathPicker')"
        @click="openPicker"
      >
        <ListTree class="size-4" />
      </button>
    </div>

    <Select v-model="condition.operator">
      <SelectTrigger>
        <SelectValue :placeholder="t('placeholders.selectOperator')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="op in operatorOptions" :key="op" :value="op">
          {{ op }}
        </SelectItem>
      </SelectContent>
    </Select>

    <div>
      <SelectTag
        v-if="isTagsPath && isListOperator && needsValue"
        v-model="valueArray"
        :items="tagOptions"
        :placeholder="t('admin.detection.value')"
      />
      <TagsInput
        v-else-if="isListOperator && needsValue"
        v-model="valueArray"
        delimiter=","
        add-on-blur
        add-on-tab
      >
        <TagsInputItem v-for="value in valueArray" :key="value" :value="value">
          <TagsInputItemText />
          <TagsInputItemDelete />
        </TagsInputItem>
        <TagsInputInput :placeholder="t('admin.detection.value')" />
      </TagsInput>
      <Input
        v-else-if="needsValue"
        v-model="condition.value"
        :placeholder="t('admin.detection.value')"
      />
    </div>

    <Button type="button" variant="ghost" size="icon" @click="$emit('remove')">
      <X class="h-4 w-4" />
    </Button>

    <PathPickerPanel
      v-model:open="pickerOpen"
      :available-sources="availableSources"
      :initial-source="condition.source || 'ticket'"
      @select="handlePathSelect"
    />
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ListTree, X } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTag,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText
} from '@shared-ui/components/ui/tags-input'
import { useTagStore } from '@/stores/tag'
import PathPickerPanel from './PathPickerPanel.vue'

const condition = defineModel({ required: true })
defineEmits(['remove'])
defineProps({
  availableSources: { type: Array, default: () => ['ticket', 'contact', 'tags'] }
})

const { t } = useI18n()
const tagStore = useTagStore()
const pickerOpen = ref(false)

const allOperators = ['=', '!=', '>', '>=', '<', '<=', 'contains', 'not contains', 'contains RegEx', 'in', 'not in', 'is null', 'is not null']
const tagOperators = ['in', 'not in']

onMounted(() => {
  tagStore.fetchTags()
})

const displayPath = (source, path) => {
  if (!source) return ''
  if (source === 'tags') return 'tags'
  return path ? `${source}.${path}` : source
}

const splitDisplayPath = (display) => {
  if (!display) return { source: '', path: '' }
  if (display === 'tags') return { source: 'tags', path: '' }
  const idx = display.indexOf('.')
  if (idx === -1) return { source: display, path: '' }
  return { source: display.slice(0, idx), path: display.slice(idx + 1) }
}

const displayPathValue = computed({
  get: () => displayPath(condition.value.source, condition.value.path),
  set: (value) => {
    const parsed = splitDisplayPath(value)
    condition.value.source = parsed.source
    condition.value.path = parsed.path
  }
})

const isTagsPath = computed(() => condition.value.source === 'tags')
const operatorOptions = computed(() => isTagsPath.value ? tagOperators : allOperators)
const isListOperator = computed(() => ['in', 'not in'].includes(condition.value.operator))
const needsValue = computed(() => !['is null', 'is not null'].includes(condition.value.operator))
const tagOptions = computed(() => (tagStore.tags || []).map((tag) => ({
  label: tag.name,
  value: tag.name,
  color: tag.color || 'grey'
})))

const valueArray = computed({
  get: () => Array.isArray(condition.value.value) ? condition.value.value : [],
  set: (value) => {
    condition.value.value = value
  }
})

watch(
  () => condition.value.source,
  (source) => {
    if (source === 'tags') {
      condition.value.path = ''
      if (!tagOperators.includes(condition.value.operator)) {
        condition.value.operator = 'in'
      }
      if (!Array.isArray(condition.value.value)) {
        condition.value.value = []
      }
    }
  },
  { immediate: true }
)

watch(
  () => condition.value.operator,
  (operator) => {
    if (['is null', 'is not null'].includes(operator)) {
      condition.value.value = null
    } else if (['in', 'not in'].includes(operator) && !Array.isArray(condition.value.value)) {
      condition.value.value = []
    } else if (!condition.value.value && !['in', 'not in'].includes(operator)) {
      condition.value.value = ''
    }
  },
  { immediate: true }
)

const openPicker = () => {
  pickerOpen.value = true
}

const handlePathSelect = (selection) => {
  condition.value.source = selection.source
  condition.value.path = selection.path
  if (selection.operator) condition.value.operator = selection.operator
  if (selection.value !== undefined) condition.value.value = selection.value
}
</script>
