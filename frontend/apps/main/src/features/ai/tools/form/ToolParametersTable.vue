<template>
  <div class="space-y-2">
    <table class="w-full text-sm table-fixed">
      <thead>
        <tr class="text-left text-muted-foreground">
          <th class="py-1 px-2 w-[20%] font-medium">
            {{ $t('admin.ai.tool.params.key') }}
          </th>
          <th class="py-1 px-2 w-[140px] font-medium">
            {{ $t('admin.ai.tool.params.type') }}
          </th>
          <th class="py-1 px-2 font-medium">
            {{ $t('admin.ai.tool.params.description') }}
          </th>
          <th class="py-1 px-2 w-20 text-center font-medium">
            {{ $t('admin.ai.tool.params.required') }}
          </th>
          <th class="py-1 w-10"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="rows.length === 0">
          <td colspan="5" class="py-2 text-xs text-muted-foreground italic">
            {{ $t('admin.ai.tool.params.empty') }}
          </td>
        </tr>
        <tr v-for="(row, idx) in rows" :key="idx">
          <td class="py-1 pr-2">
            <Input
              v-model="row.key"
              :placeholder="$t('admin.ai.tool.params.keyPlaceholder')"
              @input="emitRows"
            />
          </td>
          <td class="py-1 px-2">
            <Select :model-value="row.type || 'string'" @update:modelValue="(v) => onTypeChange(row, v)">
              <SelectTrigger class="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="t in TYPE_OPTIONS" :key="t" :value="t">{{ t }}</SelectItem>
              </SelectContent>
            </Select>
          </td>
          <td class="py-1 px-2">
            <Input v-model="row.description" @input="emitRows" />
          </td>
          <td class="py-1 px-2 text-center">
            <Checkbox :checked="!!row.required" @update:checked="(v) => onRequiredChange(row, v)" />
          </td>
          <td class="py-1 text-center">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              class="text-muted-foreground hover:text-foreground"
              @click="removeRow(idx)"
            >
              <TrashCan class="size-4" />
            </Button>
          </td>
        </tr>
      </tbody>
    </table>

    <Button type="button" variant="outline" size="sm" @click="addRow">
      <Plus class="size-4" />
      {{ t('admin.ai.function.addParameter') }}
    </Button>
  </div>
</template>

<script setup>
// ToolParametersTable — typed editor for an ai_tools row's
// input_parameters JSONB. Replaces the JSON editor in CreateOrEditTool
// for the common case (a flat list of {key, type, required, description}).
// Admins who need exotic shapes still have the JSON editor fallback.
//
// Type vocabulary mirrors internal/ai/tools/validate.Schema:
//   string, integer, number, boolean, date, array, object
import { ref, watch } from 'vue'
import { Input } from '@shared-ui/components/ui/input'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import { Button } from '@shared-ui/components/ui/button'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@shared-ui/components/ui/select'
import { TrashCan, Plus } from '@qall/icons'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const TYPE_OPTIONS = ['string', 'integer', 'number', 'boolean', 'date', 'array', 'object']

const props = defineProps({
  modelValue: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue'])

const rows = ref(normalize(props.modelValue))

watch(
  () => props.modelValue,
  (incoming) => {
    if (incoming === rows.value) return
    rows.value = normalize(incoming)
  },
  { deep: true }
)

function normalize(list) {
  return (list || []).map((r) => ({
    key: r.key || '',
    type: r.type || 'string',
    required: !!r.required,
    description: r.description || ''
  }))
}

const emitRows = () => emit('update:modelValue', rows.value.map((r) => ({ ...r })))

const onTypeChange = (row, v) => {
  row.type = v
  emitRows()
}
const onRequiredChange = (row, v) => {
  row.required = !!v
  emitRows()
}
const addRow = () => {
  rows.value.push({ key: '', type: 'string', required: false, description: '' })
  emitRows()
}
const removeRow = (idx) => {
  rows.value.splice(idx, 1)
  emitRows()
}
</script>
