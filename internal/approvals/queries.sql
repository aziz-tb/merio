-- name: insert
-- Idempotent against the partial-unique index (gated_run_trace_id WHERE
-- status='pending'): a duplicate insert for the same gated run (e.g. a Temporal
-- activity retry) conflicts and returns no row, so the write is recorded at most
-- once. The manager loads the existing pending record on conflict.
INSERT INTO agent_write_approvals (
    conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
    tool_id, tool_use_id, proposed_inputs
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (gated_run_trace_id) WHERE status = 'pending' DO NOTHING
RETURNING id, conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
          tool_id, tool_use_id, proposed_inputs, status, approved_inputs,
          rejection_reason, reviewed_by_user_id, resolved_at, created_at, updated_at;

-- name: get-pending-by-trace
SELECT id, conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
       tool_id, tool_use_id, proposed_inputs, status, approved_inputs,
       rejection_reason, reviewed_by_user_id, resolved_at, created_at, updated_at
FROM   agent_write_approvals
WHERE  gated_run_trace_id = $1 AND status = 'pending';

-- name: get-by-id
SELECT id, conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
       tool_id, tool_use_id, proposed_inputs, status, approved_inputs,
       rejection_reason, reviewed_by_user_id, resolved_at, created_at, updated_at
FROM   agent_write_approvals
WHERE  id = $1;

-- name: list-pending-by-conversation
SELECT id, conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
       tool_id, tool_use_id, proposed_inputs, status, approved_inputs,
       rejection_reason, reviewed_by_user_id, resolved_at, created_at, updated_at
FROM   agent_write_approvals
WHERE  conversation_id = $1 AND status = 'pending'
ORDER  BY created_at ASC;

-- name: resolve
-- Atomic status transition = the idempotency lock. Only a 'pending' row can be
-- resolved; the WHERE clause makes a duplicate/concurrent resolve match zero
-- rows so the write (executed only in the resuming run) can never run twice.
UPDATE agent_write_approvals
SET    status               = $2,
       approved_inputs      = $3,
       rejection_reason     = $4,
       reviewed_by_user_id  = $5,
       resolved_at          = NOW(),
       updated_at           = NOW()
WHERE  id = $1 AND status = 'pending'
RETURNING id, conversation_id, gated_run_trace_id, parent_trace_id, agent_id,
          tool_id, tool_use_id, proposed_inputs, status, approved_inputs,
          rejection_reason, reviewed_by_user_id, resolved_at, created_at, updated_at;
