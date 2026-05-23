// Normalize the two surfaces that feed AgentRunResultPanel — the
// runner's RunResponse (returned inline from /api/v1/ai/agents/:id/run)
// and the agent_execution row (returned by GET /agent-executions/latest)
// — into the panel's expected shape.
//
// The shapes diverge mostly in metric naming:
//   RunResponse.metrics.total_latency_ms ↔ agent_execution.duration_ms
//   RunResponse.iterations               ↔ derived from trace.length
//
// Both shapes share trace[], usage{}, draft, status. The panel reads
// from a single normalized shape so its template stays one file.

export function normalizeRunResponse(resp) {
  if (!resp) return null
  return {
    status: resp.status,
    draft: resp.draft ?? null,
    trace: Array.isArray(resp.trace) ? resp.trace : [],
    usage: resp.usage || {},
    duration_ms: resp.metrics?.total_latency_ms ?? 0,
    partial_reason: resp.partial_reason || '',
    iterations: resp.iterations ?? 0
  }
}

export function normalizeExecution(execution) {
  if (!execution) return null
  const trace = Array.isArray(execution.trace) ? execution.trace : []
  return {
    status: execution.status,
    draft: execution.draft ?? null,
    trace,
    usage: execution.usage || {},
    duration_ms: execution.duration_ms ?? 0,
    partial_reason: '', // not persisted to agent_executions today
    iterations: trace.length
  }
}
