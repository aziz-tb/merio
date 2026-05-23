export function humanizeToolId(id) {
  if (!id) return ''
  return String(id)
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

export function friendlyToolResultLabel({ toolId, phase, status, data, toolKind, toolName, t }) {
  if (toolKind === 'unknown') {
    return t('tools.unknown', { id: toolId || '?' })
  }

  if (toolId === 'kb_search') {
    if (phase === 'dispatched') return t('tools.kb_search.dispatched')
    if (status === 'success') {
      const n = data?.results?.length || 0
      if (n === 0) return t('tools.kb_search.completed_empty')
      return t('tools.kb_search.completed_found', n, { count: n })
    }
    return t('tools.kb_search.failed')
  }

  if (toolId === 'get_customer_data' || toolId === 'get_conversation_history') {
    if (phase === 'dispatched') return t(`tools.${toolId}.dispatched`)
    if (status === 'success') return t(`tools.${toolId}.completed`)
    return t(`tools.${toolId}.failed`)
  }

  if (toolKind === 'custom' && toolName) {
    if (phase === 'completed' && status && status !== 'success') {
      return `${toolName} ${t('tools.failedSuffix')}`
    }
    return toolName
  }

  const fallback = humanizeToolId(toolId)
  if (phase === 'completed' && status && status !== 'success') {
    return `${fallback} ${t('tools.failedSuffix')}`
  }
  return fallback
}

export function humanizeDuration(ms) {
  if (ms == null || Number.isNaN(Number(ms))) return ''
  const value = Number(ms)
  if (value < 1000) return `${Math.round(value)} ms`
  if (value < 60000) return `${(value / 1000).toFixed(1)} s`
  return `${(value / 60000).toFixed(1)} min`
}

export function shortJson(value, limit = 360) {
  if (value == null) return ''
  try {
    const text = JSON.stringify(value, null, 2)
    if (text.length <= limit) return text
    return text.slice(0, limit) + '\n…'
  } catch {
    const text = String(value)
    if (text.length <= limit) return text
    return text.slice(0, limit) + '…'
  }
}

export function queryPreview(input) {
  const query = input?.query
  if (typeof query !== 'string') return ''
  const trimmed = query.trim()
  if (!trimmed) return ''
  return trimmed.length > 80 ? `${trimmed.slice(0, 80)}…` : trimmed
}
