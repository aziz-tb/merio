<template>
  <div class="space-y-3">
    <div class="overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow class="border border-background hover:bg-background">
            <TableHead class="w-[26%]">
              {{ t('admin.customerIdentification.searchParameters.key') }}
            </TableHead>
            <TableHead class="w-[180px]">
              {{ t('admin.customerIdentification.searchParameters.datatype') }}
            </TableHead>
            <TableHead>
              {{ t('admin.customerIdentification.searchParameters.description') }}
            </TableHead>
            <TableHead class="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in rows" :key="row.id" class="border border-background hover:bg-background last:border-0">
            <TableCell class="align-top">
              <Input v-model="row.key" placeholder="contractId" @keydown.enter.prevent />
              <p v-if="errors[row.id]" class="mt-1 text-xs text-destructive">
                {{ errors[row.id] }}
              </p>
            </TableCell>
            <TableCell class="align-top">
              <Select v-model="row.datatype">
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="string">string</SelectItem>
                  <SelectItem value="number">number</SelectItem>
                  <SelectItem value="boolean">boolean</SelectItem>
                </SelectContent>
              </Select>
            </TableCell>
            <TableCell class="align-top">
              <Input v-model="row.description" placeholder="Customer email address" @keydown.enter.prevent />
            </TableCell>
            <TableCell class="align-top text-right">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                :title="t('globals.messages.remove')"
                @click="removeRow(row.id)"
              >
                <TrashCan class="size-4" />
                <span class="sr-only">{{ t('globals.messages.remove') }}</span>
              </Button>
            </TableCell>
          </TableRow>
          <TableRow v-if="rows.length === 0">
            <TableCell colspan="4" class="text-sm text-muted-foreground">
              {{ t('admin.customerIdentification.searchParameters.empty') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <Button type="button" variant="outline" size="sm" class="ml-2" @click="addRow">
      <Plus class="size-4" />
      {{ t('admin.customerIdentification.searchParameters.addAttribute') }}
    </Button>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@shared-ui/components/ui/table'
import { Plus, TrashCan } from '@qall/icons'

const rows = defineModel({ required: true })

defineProps({
  errors: { type: Object, default: () => ({}) }
})

const { t } = useI18n()

function makeID() {
  if (window.crypto?.randomUUID) return window.crypto.randomUUID()
  return `search-param-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function addRow() {
  rows.value.push({
    id: makeID(),
    key: '',
    datatype: 'string',
    description: ''
  })
}

function removeRow(id) {
  rows.value = rows.value.filter((row) => row.id !== id)
}
</script>
