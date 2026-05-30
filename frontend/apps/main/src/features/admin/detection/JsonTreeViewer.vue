<template>
  <div class="font-mono text-xs">
    <!-- Root opening brace -->
    <div v-if="isRoot" class="leading-relaxed ml-1">
      <span class="text-muted-foreground">{{ rootIsArray ? '[' : '{' }}</span>
    </div>

    <!--
      Tree contents. At the root, wrap the list in the same indent-with-border
      container that nested branches use. At deeper levels, the parent's branch
      handler already provides the wrapper, so we just render the bare list.
    -->
    <div :class="isRoot ? 'pl-2 border-l border-border/60 ml-2' : ''">
      <ul>
        <li
          v-for="node in nodes"
          :key="node.key"
          :class="[
            'relative leading-relaxed',
            isDimmed(node) ? 'opacity-30' : 'opacity-100',
            'transition-opacity'
          ]"
        >
          <!-- Empty branch: render inline as {} or [] (no chevron, no children) -->
          <template v-if="node.isBranch && node.childCount === 0">
            <div
              class="group/row flex items-baseline gap-1 rounded px-1 -mx-1 transition-colors"
              :class="{ 'bg-yellow-100/40 dark:bg-yellow-900/20': isMatch(node) }"
            >
              <span class="shrink-0 w-4" />
              <span class="font-semibold text-slate-700 dark:text-slate-300 shrink-0">
                <template v-if="parentIsArray">{{ node.key }}</template>
                <template v-else>"{{ node.key }}"</template></span><span class="text-muted-foreground shrink-0">:</span>
              <span class="text-muted-foreground ml-1">{{ node.type === 'array' ? '[]' : '{}' }}</span>
              <button
                type="button"
                class="shrink-0 opacity-0 group-hover/row:opacity-100 focus:opacity-100 rounded p-1 text-muted-foreground hover:text-foreground hover:bg-muted transition-opacity self-center"
                :title="actionTitle"
                :aria-label="actionTitle"
                @click.stop="handleAction(node.path)"
              >
                <Check v-if="copiedPath === node.path" class="size-3 text-green-600 dark:text-green-400" />
                <Copy />
              </button>
            </div>
          </template>

          <!-- Non-empty branch: expandable -->
          <template v-else-if="node.isBranch">
            <div
              class="group/row flex items-baseline gap-1 rounded px-1 -mx-1 transition-colors"
              :class="{ 'bg-yellow-100/40 dark:bg-yellow-900/20': isMatch(node) }"
            >
              <button
                type="button"
                class="shrink-0 self-start mt-1 ml-1 rounded p-0.5 hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
                :aria-expanded="isExpanded(node.path)"
                :aria-label="isExpanded(node.path) ? t('jsonTree.collapse') : t('jsonTree.expand')"
                @click="toggle(node.path)"
              >
                <ChevronDown v-if="isExpanded(node.path)" class="size-3" />
                <ChevronRight v-else class="size-3" />
              </button>

              <span class="font-semibold text-slate-700 dark:text-slate-300">
                <template v-if="parentIsArray">{{ node.key }}</template>
                <template v-else>"{{ node.key }}"</template></span><span class="text-muted-foreground">:</span>

              <template v-if="isExpanded(node.path)">
                <!-- Expanded: just the opening brace + action icon -->
                <span class="text-muted-foreground ml-1">{{ node.type === 'array' ? '[' : '{' }}</span>
                <button
                  type="button"
                  class="shrink-0 opacity-0 group-hover/row:opacity-100 focus:opacity-100 rounded p-1 text-muted-foreground hover:text-foreground hover:bg-muted transition-opacity self-center"
                  :title="actionTitle"
                  :aria-label="actionTitle"
                  @click.stop="handleAction(node.path)"
                >
                  <Check v-if="copiedPath === node.path" class="size-3 text-green-600 dark:text-green-400" />
                  <Copy v-else />
                </button>
              </template>
              <template v-else>
                <!--
                  Collapsed: { … } peek is itself a button that toggles
                  expand. The ellipsis is the natural "click to see more"
                  affordance, so make it act like one.
                -->
                <button
                  type="button"
                  class="text-muted-foreground ml-1 hover:text-foreground cursor-pointer transition-colors"
                  :title="t('jsonTree.expand')"
                  @click="toggle(node.path)"
                >
                  {{ node.type === 'array' ? '[ … ]' : '{ … }' }}
                </button>
                <button
                  type="button"
                  class="shrink-0 opacity-0 group-hover/row:opacity-100 focus:opacity-100 rounded p-1 text-muted-foreground hover:text-foreground hover:bg-muted transition-opacity self-center"
                  :title="actionTitle"
                  :aria-label="actionTitle"
                  @click.stop="handleAction(node.path)"
                >
                  <Check v-if="copiedPath === node.path" class="size-3 text-green-600 dark:text-green-400" />
                  <Copy v-else />
                </button>
              </template>
            </div>

            <!-- Children when expanded -->
            <div v-if="isExpanded(node.path)" class="pl-4 border-l border-border/60 ml-3">
              <JsonTreeViewer
                :value="node.value"
                :base-path="node.path"
                :mode="mode"
                :search-query="rootSearchQuery"
                :searchable="false"
                :is-root="false"
                :parent-is-array="node.type === 'array'"
                :expanded-paths="expandedPaths"
                :copied-path="copiedPath"
                @select-path="bubbleSelect"
                @copy-path="bubbleCopy"
                @toggle="toggle"
              />
              <div class="text-muted-foreground bg-background -ml-5">
                {{ node.type === 'array' ? ']' : '}' }}
              </div>
            </div>
          </template>

          <!-- Leaf -->
          <div
            v-else
            class="group/row rounded px-1 -mx-1 transition-colors"
            :class="{ 'bg-yellow-100/40 dark:bg-yellow-900/20': isMatch(node) }"
          >
            <div class="flex items-baseline gap-1">
              <span class="shrink-0 w-4" />
              <span class="min-w-0 flex-1">
                <span class="font-semibold text-slate-700 dark:text-slate-300">
                  <template v-if="parentIsArray">{{ node.key }}</template>
                  <template v-else>"{{ node.key }}"</template></span><span class="text-muted-foreground">:</span>
                <span :class="valueClasses(node.value)" class="ml-2 break-words">{{ formatValueDisplay(node.value) }}</span><button
                  type="button"
                  class="inline-flex items-center align-middle ml-2 opacity-0 group-hover/row:opacity-100 focus:opacity-100 rounded p-1 text-muted-foreground hover:text-foreground hover:bg-muted transition-opacity"
                  :title="actionTitle"
                  :aria-label="actionTitle"
                  @click.stop="handleAction(node.path)"
                >
                  <Check v-if="copiedPath === node.path" class="size-3 text-green-600 dark:text-green-400" />
                  <Copy v-else />
                </button>
              </span>
            </div>
          </div>
        </li>
      </ul>
    </div>

    <!-- Root closing brace -->
    <div v-if="isRoot" class="leading-relaxed">
      <span class="text-muted-foreground ml-1">{{ rootIsArray ? ']' : '}' }}</span>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search, Check, ListTree } from 'lucide-vue-next'
import { Copy, ChevronDown, ChevronRight } from '@qall/icons'

defineOptions({ name: 'JsonTreeViewer' })

const props = defineProps({
  value: { type: null, required: true },
  basePath: { type: String, default: '' },
  mode: {
    type: String,
    default: 'view',
    validator: (v) => ['view', 'picker'].includes(v)
  },
  searchable: { type: Boolean, default: true },
  searchQuery: { type: String, default: '' },
  isRoot: { type: Boolean, default: true },
  parentIsArray: { type: Boolean, default: false },
  expandedPaths: { type: Object, default: null },
  copiedPath: { type: String, default: '' }
})

const emit = defineEmits(['select-path', 'copy-path', 'toggle'])

const { t } = useI18n()

// --- Search state ---
const localSearchQuery = ref('')
const rootSearchQuery = computed(() => props.isRoot ? localSearchQuery.value : props.searchQuery)
const searchQuery = computed({
  get: () => props.isRoot ? localSearchQuery.value : props.searchQuery,
  set: (v) => { if (props.isRoot) localSearchQuery.value = v }
})
const normalizedQuery = computed(() => rootSearchQuery.value.trim().toLowerCase())

// --- Expansion state ---
const localExpandedPaths = ref(new Set())
const expandedPaths = computed(() => props.expandedPaths || localExpandedPaths.value)
const explicitlyToggled = ref(new Set())

const isExpanded = (path) => {
  if (explicitlyToggled.value.has(path)) {
    return expandedPaths.value.has(path)
  }
  return true
}

const toggle = (path) => {
  if (props.isRoot) {
    explicitlyToggled.value.add(path)
    if (expandedPaths.value.has(path)) {
      expandedPaths.value.delete(path)
    } else {
      expandedPaths.value.add(path)
    }
    localExpandedPaths.value = new Set(expandedPaths.value)
  } else {
    emit('toggle', path)
  }
}

// --- Node building ---
const rootIsArray = computed(() => Array.isArray(props.value))

const nodes = computed(() => {
  if (props.value === null || typeof props.value !== 'object') return []
  if (Array.isArray(props.value)) {
    return props.value.map((value, index) => buildNode(String(index), value))
  }
  return Object.keys(props.value).map((key) => buildNode(key, props.value[key]))
})

const buildNode = (key, value) => {
  const path = props.basePath ? `${props.basePath}.${key}` : key
  const isBranch = value !== null && typeof value === 'object'
  const type = isBranch ? (Array.isArray(value) ? 'array' : 'object') : typeOf(value)
  const childCount = isBranch
    ? (Array.isArray(value) ? value.length : Object.keys(value).length)
    : null
  return { key, value, path, isBranch, type, childCount }
}

const typeOf = (v) => {
  if (v === null) return 'null'
  if (typeof v === 'string') return 'string'
  if (typeof v === 'number') return 'number'
  if (typeof v === 'boolean') return 'boolean'
  return typeof v
}

// --- Search matching ---
const isMatch = (node) => {
  if (!normalizedQuery.value) return false
  if (node.key.toLowerCase().includes(normalizedQuery.value)) return true
  if (!node.isBranch && typeof node.value !== 'object') {
    const valStr = String(node.value ?? '').toLowerCase()
    if (valStr.includes(normalizedQuery.value)) return true
  }
  return false
}

const isDimmed = (node) => {
  if (!normalizedQuery.value) return false
  if (isMatch(node)) return false
  if (node.isBranch && branchHasMatchingDescendant(node)) return false
  return true
}

const branchHasMatchingDescendant = (node) => {
  if (!node.isBranch) return false
  const stack = [node.value]
  while (stack.length > 0) {
    const current = stack.pop()
    if (current === null || typeof current !== 'object') continue
    if (Array.isArray(current)) {
      for (let i = 0; i < current.length; i++) {
        const v = current[i]
        if (matchesValue(String(i), v)) return true
        if (v !== null && typeof v === 'object') stack.push(v)
      }
    } else {
      for (const key of Object.keys(current)) {
        const v = current[key]
        if (matchesValue(key, v)) return true
        if (v !== null && typeof v === 'object') stack.push(v)
      }
    }
  }
  return false
}

const matchesValue = (key, value) => {
  if (key.toLowerCase().includes(normalizedQuery.value)) return true
  if (value !== null && typeof value !== 'object') {
    if (String(value).toLowerCase().includes(normalizedQuery.value)) return true
  }
  return false
}

const visibleNodeCount = computed(() => {
  if (!normalizedQuery.value) return nodes.value.length
  return nodes.value.filter((n) => !isDimmed(n)).length
})

watch(normalizedQuery, (q) => {
  if (!q || !props.isRoot) return
  expandMatchingDescendants(props.value, props.basePath)
  localExpandedPaths.value = new Set(expandedPaths.value)
})

const expandMatchingDescendants = (value, prefix) => {
  if (value === null || typeof value !== 'object') return
  const entries = Array.isArray(value)
    ? value.map((v, i) => [String(i), v])
    : Object.entries(value)
  for (const [key, childValue] of entries) {
    const path = prefix ? `${prefix}.${key}` : key
    const isBranch = childValue !== null && typeof childValue === 'object'
    if (!isBranch) continue
    const fakeNode = { isBranch: true, value: childValue }
    if (isMatch({ key, value: childValue, isBranch: true }) || branchHasMatchingDescendant(fakeNode)) {
      expandedPaths.value.add(path)
      explicitlyToggled.value.add(path)
      expandMatchingDescendants(childValue, path)
    }
  }
}

// --- Value rendering ---
const valueClasses = (value) => {
  if (value === null) return 'inline-flex items-center rounded px-0.5 py-0 text-xs font-semibold uppercase tracking-wide bg-muted text-muted-foreground'
  if (typeof value === 'string') return 'text-orange-700 dark:text-orange-400'
  if (typeof value === 'number') return 'text-blue-700 dark:text-blue-400 font-semibold'
  if (typeof value === 'boolean') return 'text-purple-700 dark:text-purple-400 font-semibold'
  return 'text-foreground'
}

const formatValueDisplay = (value) => {
  if (value === null) return 'null'
  if (typeof value === 'string') return `"${value}"`
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

// --- Action ---
const actionIcon = computed(() => props.mode === 'picker' ? ListTree : Copy)
const actionTitle = computed(() => props.mode === 'picker'
  ? t('jsonTree.selectPath')
  : t('jsonTree.copyPath'))

const localCopiedPath = ref('')
const copiedPath = computed(() => props.isRoot ? localCopiedPath.value : props.copiedPath)

const handleAction = async (path) => {
  if (props.mode === 'picker') {
    emit('select-path', path)
  } else {
    try {
      await navigator.clipboard.writeText(path)
    } catch (err) {
      // Clipboard API may fail in non-secure contexts; ignore.
    }
    if (props.isRoot) {
      localCopiedPath.value = path
      setTimeout(() => {
        if (localCopiedPath.value === path) localCopiedPath.value = ''
      }, 1500)
    }
    emit('copy-path', path)
  }
}

const bubbleSelect = (path) => emit('select-path', path)
const bubbleCopy = (path) => {
  if (props.isRoot) {
    localCopiedPath.value = path
    setTimeout(() => {
      if (localCopiedPath.value === path) localCopiedPath.value = ''
    }, 1500)
  }
  emit('copy-path', path)
}
</script>