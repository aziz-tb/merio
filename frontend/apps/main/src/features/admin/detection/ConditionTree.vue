<template>
  <div class="space-y-3 rounded border border-border p-3">
    <div class="flex items-center justify-between gap-3">
      <Select v-model="tree.op">
        <SelectTrigger class="w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="AND">{{ $t('admin.automation.and') }}</SelectItem>
          <SelectItem value="OR">{{ $t('admin.automation.or') }}</SelectItem>
        </SelectContent>
      </Select>

      <div class="flex gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          :disabled="isNotFull"
          @click="addCondition"
        >
          <Plus class="h-4 w-4" />
          {{ $t('actions.addCondition') }}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          :disabled="isNotFull"
          @click="addGroup"
        >
          <FolderPlus class="h-4 w-4" />
          {{ $t('admin.detection.addGroup') }}
        </Button>
        <Button v-if="removable" type="button" variant="ghost" size="icon" @click="$emit('remove')">
          <X class="h-4 w-4" />
        </Button>
      </div>
    </div>

    <div class="space-y-3">
      <template v-for="(child, index) in tree.children" :key="index">
        <ConditionTree
          v-if="child.tree"
          v-model="child.tree"
          :available-sources="availableSources"
          removable
          @remove="removeChild(index)"
        />
        <ConditionLeaf
          v-else-if="child.leaf"
          v-model="child.leaf"
          :available-sources="availableSources"
          @remove="removeChild(index)"
        />
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, watch } from 'vue'
import { FolderPlus, Plus, X } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@shared-ui/components/ui/select'
import ConditionLeaf from './ConditionLeaf.vue'

defineOptions({ name: 'ConditionTree' })

const tree = defineModel({ required: true })
defineEmits(['remove'])
defineProps({
  availableSources: { type: Array, default: () => ['ticket', 'contact', 'tags'] },
  removable: { type: Boolean, default: false }
})

const isNotFull = computed(() => tree.value.op === 'NOT' && tree.value.children.length >= 1)

watch(
  () => tree.value.op,
  (op) => {
    if (op === 'NOT' && tree.value.children.length > 1) {
      tree.value.children = tree.value.children.slice(0, 1)
    }
  }
)

const addCondition = () => {
  tree.value.children.push({
    leaf: {
      source: 'ticket',
      path: '',
      operator: '=',
      value: ''
    }
  })
}

const addGroup = () => {
  tree.value.children.push({
    tree: {
      op: 'AND',
      children: []
    }
  })
}

const removeChild = (index) => {
  tree.value.children.splice(index, 1)
}
</script>
