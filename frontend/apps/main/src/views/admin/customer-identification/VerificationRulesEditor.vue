<template>
  <div class="space-y-3">
    <div v-for="group in groups" :key="group.id" class="rounded-lg border border-input bg-background">
      <Collapsible :open="group.open" @update:open="(v) => (group.open = v)">
        <div class="flex items-center justify-between gap-3 p-3">
          <CollapsibleTrigger as-child>
            <button type="button" class="flex min-w-0 flex-1 items-center gap-2 text-left">
              <ChevronRight
                :class="[
                  'size-4 shrink-0 text-muted-foreground transition-transform',
                  group.open ? 'rotate-90' : ''
                ]"
              />
              <span class="truncate text-sm font-medium">
                {{ group.label || t('admin.customerIdentification.verificationRules.untitledGroup') }}
              </span>
            </button>
          </CollapsibleTrigger>

          <div class="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              :title="t('globals.messages.edit')"
              @click="group.open = true"
            >
              <Pen class="size-4" />
              <span class="sr-only">{{ t('globals.messages.edit') }}</span>
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              :title="t('globals.messages.delete')"
              @click="removeGroup(group.id)"
            >
              <TrashCan class="size-4" />
              <span class="sr-only">{{ t('globals.messages.delete') }}</span>
            </Button>
          </div>
        </div>

        <CollapsibleContent>
          <div class="space-y-5 border-t border-border p-4">
            <p v-if="groupErrors[group.id]" class="text-xs text-destructive">
              {{ groupErrors[group.id] }}
            </p>
            <div
              v-for="warning in warningsByGroup(group.id)"
              :key="warning.criterion"
              class="rounded-md border border-orange-200 bg-orange-50 px-3 py-2 text-xs text-orange-900 dark:border-orange-900/50 dark:bg-orange-950/30 dark:text-orange-200"
            >
              {{ t('admin.customerIdentification.validation.unknownCriterion', {
                group: group.label || t('admin.customerIdentification.verificationRules.untitledGroup'),
                criterion: warning.criterion
              }) }}
            </div>

            <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_160px]">
              <div class="space-y-2">
                <Label>{{ t('admin.customerIdentification.verificationRules.groupLabel') }}</Label>
                <Input v-model="group.label" @keydown.enter.prevent />
              </div>
              <div class="space-y-2">
                <Label>{{ t('admin.customerIdentification.verificationRules.minCount') }}</Label>
                <Input
                  type="number"
                  min="1"
                  step="1"
                  :model-value="group.min_count"
                  @input="(e) => setMinCount(group, e.target.value)"
                />
              </div>
            </div>

            <div class="space-y-2">
              <Label>{{ t('admin.customerIdentification.verificationRules.standardCriteria') }}</Label>
              <div class="grid gap-2 rounded-md border border-input p-3 md:grid-cols-2">
                <label
                  v-for="criterion in standardCriteria"
                  :key="criterion"
                  class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted"
                >
                  <Checkbox
                    :checked="group.standard_criteria.includes(criterion)"
                    @update:checked="() => toggleStandardCriterion(group, criterion)"
                  />
                  <span>{{ t(`admin.customerIdentification.standardCriterion.${criterion}`) }}</span>
                </label>
              </div>
            </div>

            <div class="space-y-3">
              <div class="flex items-center justify-between gap-3">
                <Label>{{ t('admin.customerIdentification.verificationRules.customCriteria') }}</Label>
                <Button type="button" variant="outline" size="sm" @click="addCustomCriterion(group)">
                  <Plus class="size-4" />
                  {{ t('admin.customerIdentification.verificationRules.addCustomCriterion') }}
                </Button>
              </div>

              <div v-if="group.custom_criteria.length === 0" class="rounded-md border border-dashed border-input p-3 text-sm text-muted-foreground">
                {{ t('admin.customerIdentification.customCriterion.empty') }}
              </div>

              <div
                v-for="criterion in group.custom_criteria"
                :key="criterion.id"
                class="rounded-md border border-input"
              >
                <Collapsible :open="criterion.open" @update:open="(v) => (criterion.open = v)">
                  <div class="flex items-center justify-between gap-3 p-3">
                    <CollapsibleTrigger as-child>
                      <button type="button" class="flex min-w-0 flex-1 items-center gap-2 text-left">
                        <ChevronRight
                          :class="[
                            'size-4 shrink-0 text-muted-foreground transition-transform',
                            criterion.open ? 'rotate-90' : ''
                          ]"
                        />
                        <span class="truncate text-sm font-medium">
                          {{ criterion.label || t('admin.customerIdentification.customCriterion.untitled') }}
                        </span>
                      </button>
                    </CollapsibleTrigger>
                    <div class="flex items-center gap-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        :title="t('globals.messages.edit')"
                        @click="criterion.open = true"
                      >
                        <Pencil class="size-4" />
                        <span class="sr-only">{{ t('globals.messages.edit') }}</span>
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        :title="t('globals.messages.delete')"
                        @click="removeCustomCriterion(group, criterion.id)"
                      >
                        <TrashCan class="size-4" />
                        <span class="sr-only">{{ t('globals.messages.delete') }}</span>
                      </Button>
                    </div>
                  </div>

                  <CollapsibleContent>
                    <div class="space-y-4 border-t border-border p-4">
                      <p v-if="customErrors[criterion.id]" class="text-xs text-destructive">
                        {{ customErrors[criterion.id] }}
                      </p>

                      <div class="grid gap-4 md:grid-cols-2">
                        <div class="space-y-2">
                          <Label>{{ t('admin.customerIdentification.customCriterion.label') }}</Label>
                          <Input v-model="criterion.label" @keydown.enter.prevent />
                        </div>
                        <div class="space-y-2">
                          <Label>{{ t('admin.customerIdentification.customCriterion.key') }}</Label>
                          <Input v-model="criterion.key" placeholder="extra_data.malo" @keydown.enter.prevent />
                          <p class="text-xs text-muted-foreground">
                            {{ t('admin.customerIdentification.customCriterion.keyHelp') }}
                          </p>
                        </div>
                      </div>

                      <div class="space-y-2">
                        <Label>{{ t('admin.customerIdentification.customCriterion.reason') }}</Label>
                        <Input v-model="criterion.reason" placeholder="Value {value} was not mentioned." @keydown.enter.prevent />
                        <p class="text-xs text-muted-foreground">
                          {{ t('admin.customerIdentification.customCriterion.reasonHelp') }}
                        </p>
                      </div>

                      <div class="space-y-2">
                        <Label>{{ t('admin.customerIdentification.customCriterion.searchIn') }}</Label>
                        <div class="flex flex-wrap gap-2">
                          <label
                            v-for="target in searchTargets"
                            :key="target"
                            class="flex cursor-pointer items-center gap-2 rounded-md border border-input px-3 py-2 text-sm hover:bg-muted"
                          >
                            <Checkbox
                              :checked="criterion.search_in.includes(target)"
                              @update:checked="() => toggleSearchTarget(criterion, target)"
                            />
                            <span>{{ t(`admin.customerIdentification.customCriterion.searchIn.${target}`) }}</span>
                          </label>
                        </div>
                      </div>
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              </div>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>

    <Button type="button" variant="outline" size="sm" @click="addGroup">
      <Plus class="size-4" />
      {{ t('admin.customerIdentification.verificationRules.addGroup') }}
    </Button>
  </div>
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { TrashCan, Plus, ChevronRight, Pen } from '@qall/icons'
import { Button } from '@shared-ui/components/ui/button'
import { Checkbox } from '@shared-ui/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '@shared-ui/components/ui/collapsible'
import { Input } from '@shared-ui/components/ui/input'
import { Label } from '@shared-ui/components/ui/label'

const groups = defineModel({ required: true })

const props = defineProps({
  standardCriteria: { type: Array, required: true },
  groupErrors: { type: Object, default: () => ({}) },
  customErrors: { type: Object, default: () => ({}) },
  warnings: { type: Array, default: () => [] }
})

const { t } = useI18n()
const searchTargets = ['sender', 'subject', 'message']

function makeID(prefix) {
  if (window.crypto?.randomUUID) return window.crypto.randomUUID()
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function addGroup() {
  groups.value.push({
    id: makeID('group'),
    label: '',
    min_count: 1,
    standard_criteria: [],
    unknown_criteria: [],
    custom_criteria: [],
    open: true
  })
}

function removeGroup(id) {
  groups.value = groups.value.filter((group) => group.id !== id)
}

function setMinCount(group, value) {
  const next = Number(value)
  group.min_count = Number.isFinite(next) ? next : 1
}

function toggleStandardCriterion(group, criterion) {
  if (group.standard_criteria.includes(criterion)) {
    group.standard_criteria = group.standard_criteria.filter((item) => item !== criterion)
  } else {
    group.standard_criteria = [...group.standard_criteria, criterion]
  }
}

function addCustomCriterion(group) {
  group.custom_criteria.push({
    id: makeID('custom-criterion'),
    label: '',
    key: '',
    reason: '',
    search_in: ['message'],
    open: true
  })
  group.open = true
}

function removeCustomCriterion(group, id) {
  group.custom_criteria = group.custom_criteria.filter((criterion) => criterion.id !== id)
}

function toggleSearchTarget(criterion, target) {
  if (criterion.search_in.includes(target)) {
    criterion.search_in = criterion.search_in.filter((item) => item !== target)
  } else {
    criterion.search_in = [...criterion.search_in, target]
  }
}

function warningsByGroup(groupId) {
  return props.warnings.filter((warning) => warning.groupId === groupId)
}
</script>
