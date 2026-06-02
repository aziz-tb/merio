import { formatDateTime } from './date'

export function humanizeKey (key) {
  if (!key) return ''

  let label = String(key).replace(/_/g, ' ')
  label = label.replace(/([a-z0-9])([A-Z])/g, '$1 $2')
  label = label.replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')

  const acronyms = new Set(['id', 'url', 'uri', 'ip', 'iban', 'erp'])
  return label
    .split(' ')
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase()
      if (acronyms.has(lower)) return lower.toUpperCase()
      return lower.charAt(0).toUpperCase() + lower.slice(1)
    })
    .join(' ')
}

export function formatValue (value) {
  if (value === null || value === undefined || value === '') {
    return { kind: 'simple', display: '—' }
  }

  if (typeof value === 'boolean') {
    return { kind: 'simple', display: value ? 'Yes' : 'No' }
  }

  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}/.test(value)) {
    const display = formatDateTime(value)
    if (display) return { kind: 'simple', display }
  }

  if (Array.isArray(value)) {
    if (value.length === 0) return { kind: 'simple', display: '—' }
    if (value.every((item) => item === null || typeof item !== 'object')) {
      const display = value
        .map((item) => (item == null ? '' : String(item)))
        .filter(Boolean)
        .join(', ')
      return { kind: 'simple', display: display || '—' }
    }
    return { kind: 'array-objects', display: `${value.length} items`, raw: value }
  }

  if (typeof value === 'object') {
    if (Object.keys(value).length === 0) return { kind: 'simple', display: '—' }
    return { kind: 'nested-object', display: 'Object', raw: value }
  }

  return { kind: 'simple', display: String(value) }
}

export const SKIP_FIELDS_CUSTOMER = new Set([
  'customer_id',
  'contracts',
  'contract_ids',
  'agent_preview',
  'agentPreview',
  'raw_data',
  'rawData',
  'tab_preview',
  'tabPreview',
  'extra_data',
  'created_at',
  'updated_at',
  'last_synced_at'
])

export const SKIP_FIELDS_CONTRACT = new Set([
  'contract_id',
  'agent_preview',
  'agentPreview',
  'erp_urls',
  'erpUrls',
  'erpURLs',
  'raw_data',
  'rawData',
  'tab_preview',
  'tabPreview',
  'extra_data',
  'created_at',
  'updated_at',
  'last_synced_at'
])

export const SKIP_FIELDS = SKIP_FIELDS_CUSTOMER

export const VISIBLE_FIELD_LIMIT = 15

export function orderFields (record, skipFields = SKIP_FIELDS) {
  if (!record || typeof record !== 'object' || Array.isArray(record)) return []
  return Object.keys(record).filter((key) => !skipFields.has(key))
}
