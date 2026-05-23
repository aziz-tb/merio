-- name: insert
INSERT INTO ai_operations (
    operation_type, conversation_id, triggered_by, triggered_by_user_id,
    trace_id, status, error_message, payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: get-by-id
SELECT id, created_at, operation_type, conversation_id, triggered_by,
       triggered_by_user_id, trace_id, status, error_message, payload
FROM   ai_operations
WHERE  id = $1;

-- name: list-for-conversation
SELECT id, created_at, operation_type, conversation_id, triggered_by,
       triggered_by_user_id, trace_id, status, error_message, payload
FROM   ai_operations
WHERE  conversation_id = $1
ORDER  BY created_at DESC, id DESC
LIMIT  $2;

-- name: list-by-type-for-conversation
SELECT id, created_at, operation_type, conversation_id, triggered_by,
       triggered_by_user_id, trace_id, status, error_message, payload
FROM   ai_operations
WHERE  conversation_id = $1 AND operation_type = $2
ORDER  BY created_at DESC, id DESC
LIMIT  $3;
