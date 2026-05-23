import { friendlyToolResultLabel } from './toolLabels'

const HIDDEN_TOOL_KIND = 'runner'

const toTime = (value) => {
  if (!value) return 0
  const ts = new Date(value).getTime()
  return Number.isFinite(ts) ? ts : 0
}

const kbSourceFromResult = (result) => {
  if (!result || typeof result !== 'object') return null
  if (result.article_id == null) return null
  return {
    articleId: result.article_id,
    title: result.title || `Article #${result.article_id}`,
    status: result.status || '',
    bodyExcerpt: result.body_excerpt || '',
  }
}

export function extractKnowledgeSourcesFromTrace(trace = []) {
  const seen = new Map()
  for (const turn of trace) {
    for (const call of turn.tool_calls || []) {
      if (call.tool_kind === HIDDEN_TOOL_KIND) continue
      if (call.tool_id !== 'kb_search' || call.status !== 'success') continue
      const results = call.data?.results || []
      for (const result of results) {
        const source = kbSourceFromResult(result)
        if (!source || seen.has(source.articleId)) continue
        seen.set(source.articleId, source)
      }
    }
  }
  return Array.from(seen.values())
}

export function extractKnowledgeSourcesFromStream(events = []) {
  const seen = new Map()
  for (const event of events) {
    if (event?.type !== 'tool.completed') continue
    const payload = event.payload || {}
    if (payload.tool_kind === HIDDEN_TOOL_KIND) continue
    if (payload.tool_id !== 'kb_search' || payload.status !== 'success') continue
    const results = payload.data?.results || []
    for (const result of results) {
      const source = kbSourceFromResult(result)
      if (!source || seen.has(source.articleId)) continue
      seen.set(source.articleId, source)
    }
  }
  return Array.from(seen.values())
}

export function buildActivityEventsFromTrace(trace = [], t) {
  const items = []
  for (const [turnIndex, turn] of trace.entries()) {
    const thinkingBlocks = turn.thinking_blocks ?? []
    const thinkingText = thinkingBlocks
      .filter((b) => b.type === 'thinking' && b.thinking)
      .map((b) => b.thinking)
      .join('\n\n')
    const hasRedactedThinking = thinkingBlocks.some((b) => b.type === 'redacted_thinking')

    items.push({
      id: `trace-thinking-${turnIndex}`,
      type: 'thinking',
      turnIndex,
      durationMs: turn.latency_ms ?? 0,
      pending: false,
      thinkingText,
      hasRedactedThinking,
    })

    const toolRows = []
    for (const [idx, call] of (turn.tool_calls || []).entries()) {
      if (call.tool_kind === HIDDEN_TOOL_KIND) continue
      const dispatchedLabel = friendlyToolResultLabel({
        toolId: call.tool_id,
        phase: 'dispatched',
        status: call.status,
        data: call.data,
        toolKind: call.tool_kind,
        toolName: call.tool_name || '',
        t,
      })
      const completedLabel = friendlyToolResultLabel({
        toolId: call.tool_id,
        phase: 'completed',
        status: call.status,
        data: call.data,
        toolKind: call.tool_kind,
        toolName: call.tool_name || '',
        t,
      })
      toolRows.push({
        id: `trace-tool-dispatched-${turnIndex}-${idx}`,
        type: 'tool',
        phase: 'dispatched',
        turnIndex,
        toolId: call.tool_id,
        toolKind: call.tool_kind,
        toolName: call.tool_name || '',
        label: dispatchedLabel,
        input: call.input || {},
        data: call.data,
        error: call.error || '',
        query: call.input?.query || '',
        sortTime: toTime(call.started_at),
      })
      toolRows.push({
        id: `trace-tool-completed-${turnIndex}-${idx}`,
        type: 'tool',
        phase: 'completed',
        turnIndex,
        toolId: call.tool_id,
        toolKind: call.tool_kind,
        toolName: call.tool_name || '',
        label: completedLabel,
        status: call.status || '',
        input: call.input || {},
        data: call.data,
        error: call.error || '',
        query: call.input?.query || '',
        durationMs: call.duration_ms ?? 0,
        sortTime: toTime(call.finished_at),
      })
    }
    toolRows.sort((a, b) => a.sortTime - b.sortTime)
    items.push(...toolRows)
  }
  return items
}

export function buildActivityEventsFromStream(events = [], t) {
  const items = []
  let pendingThinking = null

  const flushThinking = (nextTimestamp) => {
    if (!pendingThinking) return
    const start = toTime(pendingThinking.emittedAt)
    const finish = toTime(nextTimestamp)
    items.push({
      id: pendingThinking.id,
      type: 'thinking',
      turnIndex: pendingThinking.turnIndex,
      durationMs: start > 0 && finish > start ? finish - start : null,
      pending: false,
    })
    pendingThinking = null
  }

  for (const event of events) {
    const payload = event?.payload || {}
    if (event?.type === 'turn.completed' || payload.tool_kind === HIDDEN_TOOL_KIND) {
      continue
    }

    if (event?.type === 'turn.started') {
      flushThinking(event.emitted_at)
      pendingThinking = {
        id: event.id || `thinking-${items.length}`,
        turnIndex: payload.turn_index ?? 0,
        emittedAt: event.emitted_at,
      }
      continue
    }

    if (pendingThinking) {
      flushThinking(event?.emitted_at)
    }

    if (event?.type === 'tool.dispatched' || event?.type === 'tool.completed') {
      const label = friendlyToolResultLabel({
        toolId: payload.tool_id,
        phase: event.type === 'tool.dispatched' ? 'dispatched' : 'completed',
        status: payload.status,
        data: payload.data,
        toolKind: payload.tool_kind,
        toolName: payload.tool_name || '',
        t,
      })
      items.push({
        id: event.id || `${event.type}-${items.length}`,
        type: 'tool',
        phase: event.type === 'tool.dispatched' ? 'dispatched' : 'completed',
        turnIndex: payload.turn_index ?? 0,
        toolId: payload.tool_id || '',
        toolKind: payload.tool_kind || '',
        toolName: payload.tool_name || '',
        label,
        status: payload.status || '',
        input: payload.input || {},
        data: payload.data,
        error: payload.error || '',
        query: payload.input?.query || '',
        durationMs: payload.duration_ms ?? null,
      })
      continue
    }

    if (event?.type === 'draft.committed') {
      items.push({
        id: event.id || `draft-${items.length}`,
        type: 'draft',
        turnIndex: payload.turn_index ?? 0,
      })
    }
  }

  if (pendingThinking) {
    items.push({
      id: pendingThinking.id,
      type: 'thinking',
      turnIndex: pendingThinking.turnIndex,
      durationMs: null,
      pending: true,
    })
  }

  return items
}
