// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { createPinia } from 'pinia'
import DetectionBuilder from './DetectionBuilder.vue'
import ConditionLeaf from './ConditionLeaf.vue'
import TagMultiSelect from './TagMultiSelect.vue'

vi.mock('@/api', () => ({
  default: {
    getTags: vi.fn(async () => ({
      data: {
        data: [
          { id: 1, name: 'billing' },
          { id: 2, name: 'vip' }
        ]
      }
    }))
  }
}))

const mountedApps = []

const messages = {
  en: {
    'actions.addCondition': 'Add condition',
    'admin.automation.and': 'AND',
    'admin.automation.or': 'OR',
    'admin.detection.addGroup': 'Add group',
    'admin.detection.aiPrompt': 'AI prompt',
    'admin.detection.aiPromptPlaceholder': 'Describe what should be detected.',
    'admin.detection.mode': 'Detection mode',
    'admin.detection.mode.aiOnly': 'AI only',
    'admin.detection.mode.manualOnly': 'Manual only',
    'admin.detection.mode.rulesAndAi': 'Rules and AI',
    'admin.detection.mode.rulesOnly': 'Rules only',
	    'admin.detection.mode.rulesOrAi': 'Rules or AI',
	    'admin.detection.path': 'Path',
	    'admin.detection.pathPicker': 'Path picker',
	    'admin.detection.selectMode': 'Select mode',
    'admin.detection.selectSource': 'Select source',
    'admin.detection.source.tags': 'Tags',
    'admin.detection.source.ticket': 'Ticket',
    'admin.detection.value': 'Value',
    'globals.messages.noResultsFound': 'No results found',
    'globals.messages.search': 'Search',
    'placeholders.selectOperator': 'Select operator'
  }
}

function mount(component, props = {}) {
  const root = document.createElement('div')
  document.body.appendChild(root)
  const app = createApp(component, props)
  app.use(createPinia())
  app.use(createI18n({ legacy: false, locale: 'en', messages }))
  app.mount(root)
  mountedApps.push({ app, root })
  return root
}

afterEach(() => {
  for (const { app, root } of mountedApps.splice(0)) {
    app.unmount()
    root.remove()
  }
})

describe('DetectionBuilder', () => {
  it('renders ConditionTree for rules_only mode', () => {
    const root = mount(DetectionBuilder, {
      modelValue: {
        mode: 'rules_only',
        rules: { op: 'AND', children: [] },
        ai: { prompt: '' }
      }
    })

    expect(root.textContent).toContain('Add condition')
  })

  it('renders AIPromptEditor when mode includes AI', () => {
    const root = mount(DetectionBuilder, {
      modelValue: {
        mode: 'ai_only',
        rules: { op: 'AND', children: [] },
        ai: { prompt: '' }
      }
    })

    expect(root.querySelector('textarea')?.getAttribute('placeholder')).toBe('Describe what should be detected.')
  })
})

describe('ConditionLeaf', () => {
  it('hides value input for null operators', () => {
    const root = mount(ConditionLeaf, {
      modelValue: {
        source: 'ticket',
        path: 'subject',
        operator: 'is null',
        value: null
      }
    })

    const valueInputs = [...root.querySelectorAll('input')]
      .filter((input) => input.getAttribute('placeholder') === 'Value')
    expect(valueInputs).toHaveLength(0)
  })
})

describe('TagMultiSelect', () => {
  it('filters by search', async () => {
    const root = mount(TagMultiSelect, { modelValue: [] })
    await nextTick()
    await Promise.resolve()
    await nextTick()

    const input = root.querySelector('input')
    input.value = 'vip'
    input.dispatchEvent(new Event('input'))
    await nextTick()

    expect(root.textContent).toContain('vip')
    expect(root.textContent).not.toContain('billing')
  })
})
