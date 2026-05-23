-- name: insert-execution
-- Records an audit row for a completed (or failed-to-dispatch) script
-- execution. The runner calls this on every Execute regardless of
-- outcome — successful runs, script failures, and dispatch failures
-- all leave a trail.
INSERT INTO tool_executions (
    execution_id,
    tool_name,
    invoker_user_id,
    conversation_id,
    tool_depth,
    language,
    started_at,
    finished_at,
    duration_ms,
    kind,
    result_code,
    message,
    data,
    warm_pool_hit,
    trace_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
);
