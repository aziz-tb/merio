<template>
  <div class="relative inline-flex max-w-full min-w-0 items-center">
    <button
      v-if="!editing"
      type="button"
      :class="cn(
        'rounded text-left transition-colors hover:bg-muted/40',
        variant === 'heading'
          ? '-mx-1 px-1 text-xl font-semibold'
          : '-mx-1 px-1 text-sm'
      )"
      @click="startEdit"
    >
      <span v-if="modelValue" class="text-foreground">{{ modelValue }}</span>
      <span v-else class="italic text-muted-foreground">{{ placeholder }}</span>
    </button>

    <Input
      v-else
      ref="inputRef"
      v-model="draft"
      :placeholder="placeholder"
      :class="cn(
        variant === 'heading'
          ? '-mx-1 h-auto px-1 py-0 text-xl font-semibold'
          : 'h-7 text-sm'
      )"
      @blur="commit"
      @keydown.enter.prevent="commit"
      @keydown.esc.prevent="cancel"
    />

    <Transition name="fade">
      <span
        v-if="showSaved"
        class="ml-1 inline-flex items-center text-green-500"
      >
        <Check class="size-3" />
      </span>
    </Transition>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Check } from '@qall/icons'
import { Input } from '@shared-ui/components/ui/input'
import { cn } from '@shared-ui/lib/utils.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  variant: { type: String, default: 'default' }
})

const emit = defineEmits(['update:modelValue', 'commit'])

const editing = ref(false)
const draft = ref('')
const inputRef = ref(null)
const showSaved = ref(false)
const pendingSavedValue = ref(null)
let savedTimer = null

watch(() => props.modelValue, (value) => {
  if (!editing.value) draft.value = value || ''
  if (pendingSavedValue.value !== null && normalizeForCompare(value) === pendingSavedValue.value) {
    showSaved.value = true
    pendingSavedValue.value = null
    clearTimeout(savedTimer)
    savedTimer = setTimeout(() => {
      showSaved.value = false
    }, 1500)
  }
})

const startEdit = async () => {
  draft.value = props.modelValue || ''
  editing.value = true
  await nextTick()
  inputRef.value?.$el?.focus?.()
  inputRef.value?.$el?.select?.()
}

const commit = () => {
  if (!editing.value) return
  editing.value = false
  if (draft.value !== props.modelValue) {
    pendingSavedValue.value = normalizeForCompare(draft.value)
    emit('update:modelValue', draft.value)
    emit('commit', draft.value)
  }
}

const cancel = () => {
  editing.value = false
  draft.value = props.modelValue || ''
  pendingSavedValue.value = null
}

onBeforeUnmount(() => clearTimeout(savedTimer))

function normalizeForCompare (value) {
  return String(value || '').trim().replace(/\s+/g, ' ')
}
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 200ms;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
