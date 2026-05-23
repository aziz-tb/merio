-- name: insert-usage
-- One row per LLM provider call. Idempotency guard is the partial
-- unique index (trace_id, call_sequence_in_trace, created_at) — a
-- retry of the same logical call uses the same created_at and trips
-- the constraint, which the manager swallows.
INSERT INTO ai_usage (
    created_at,
    trace_id,
    call_sequence_in_trace,
    inbox_id,
    conversation_id,
    agent_id,
    agent_execution_id,
    resolution_attempt_id,
    user_id,
    provider,
    model,
    preset_slug,
    purpose,
    stop_reason,
    input_tokens,
    output_tokens,
    cached_input_tokens,
    cache_markers_used,
    input_cost_usd,
    output_cost_usd,
    cached_input_cost_usd,
    total_cost_usd,
    latency_ms,
    provider_latency_ms,
    retry_count,
    outcome,
    error_class,
    error_message,
    provider_request_id,
    provider_metadata,
    request_payload,
    response_payload,
    structured_output
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
    $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
    $31, $32, $33
);
