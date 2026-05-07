-- name: start
INSERT INTO worklogs (conversation_id, user_id, started_at, action_type, context)
VALUES ($1, $2, NOW(), $3, $4)
RETURNING id;

-- name: start-by-uuid
-- Resolves the conversation by UUID inline. Returns 0 rows (sql.ErrNoRows)
-- if the UUID doesn't match — caller treats that as a no-op skip.
INSERT INTO worklogs (conversation_id, user_id, started_at, action_type, context)
SELECT id, $2, NOW(), $3, $4 FROM conversations WHERE uuid = $1
RETURNING id;

-- name: end
-- Shallow-merge `extra` into context using Postgres `||` operator semantics:
-- right-side wins on key conflict. Caller passes a flat JSON object; nested
-- merge is not performed.
-- The `AND ended_at IS NULL` guard makes a second End call a silent no-op.
UPDATE worklogs
SET    ended_at         = NOW(),
       duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM (NOW() - started_at))::int),
       context          = COALESCE(context, '{}'::jsonb) || COALESCE($2::jsonb, '{}'::jsonb)
WHERE  id = $1
  AND  ended_at IS NULL;

-- name: get-by-conversation
SELECT id, conversation_id, user_id, started_at, ended_at, duration_seconds,
       action_type, automation_level, context, created_at
FROM   worklogs
WHERE  conversation_id = $1
ORDER  BY started_at DESC
LIMIT  $2 OFFSET $3;

-- name: get-by-user
SELECT id, conversation_id, user_id, started_at, ended_at, duration_seconds,
       action_type, automation_level, context, created_at
FROM   worklogs
WHERE  user_id = $1
ORDER  BY started_at DESC
LIMIT  $2 OFFSET $3;
