-- name: insert-execution
INSERT INTO agent_executions (
    conversation_id, agent_id, source, resolution_attempt_id,
    trace_id, status, draft, trace, usage, duration_ms,
    run_by_user_id, agent_updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, conversation_id, agent_id, source, resolution_attempt_id,
          trace_id, format_version, status, draft, trace, usage, duration_ms,
          run_by_user_id, run_at, agent_updated_at, created_at, updated_at;

-- name: get-latest-execution-by-trace
-- Loads the most recent run for a trace id (the approval resume reconstructs
-- context from the gated run's persisted trace via this).
SELECT id, conversation_id, agent_id, source, resolution_attempt_id,
       trace_id, format_version, status, draft, trace, usage, duration_ms,
       run_by_user_id, run_at, agent_updated_at, created_at, updated_at
FROM   agent_executions
WHERE  trace_id = $1
ORDER  BY id DESC
LIMIT  1;

-- name: list-by-agent
-- Past runs for an agent's History tab. Paginated by an opaque
-- before-cursor (run_at, id) so newer runs surface in stable order
-- even when timestamps tie. Mirrors the cursor pattern used by the
-- conversation-scoped agent-executions routes.
--
--   $1 agent_id        INT     required
--   $2 status_filter   TEXT    NULL or '' = no status filter
--   $3 search_query    TEXT    NULL or '' = no draft ILIKE filter
--   $4 before_run_at   TIMESTAMPTZ  NULL = first page
--   $5 before_id       BIGINT  NULL = first page
--   $6 page_limit      INT
--
-- Excludes 'discarded' outcomes per the conversation-scoped rule —
-- discarded runs don't surface to operators anywhere.
SELECT e.id, e.conversation_id, e.agent_id, e.source, e.resolution_attempt_id,
       e.trace_id, e.format_version, e.status, e.draft, e.trace, e.usage, e.duration_ms,
       e.run_by_user_id, e.run_at, e.agent_updated_at, e.created_at, e.updated_at
FROM   agent_executions e
LEFT JOIN agent_execution_outcomes o ON o.agent_execution_id = e.id
WHERE  e.agent_id = $1
  AND  ($2::TEXT IS NULL OR $2 = '' OR e.status = $2)
  AND  ($3::TEXT IS NULL OR $3 = '' OR e.draft ILIKE '%' || $3 || '%')
  AND  ($4::TIMESTAMPTZ IS NULL OR (e.run_at, e.id) < ($4, $5))
  AND  (o.outcome IS NULL OR o.outcome <> 'discarded')
ORDER BY e.run_at DESC, e.id DESC
LIMIT  $6;

-- name: get-execution-by-id
SELECT id, conversation_id, agent_id, source, resolution_attempt_id,
       trace_id, format_version, status, draft, trace, usage, duration_ms,
       run_by_user_id, run_at, agent_updated_at, created_at, updated_at
FROM   agent_executions
WHERE  id = $1;

-- name: latest-execution-for-conversation
SELECT id, conversation_id, agent_id, source, resolution_attempt_id,
       trace_id, format_version, status, draft, trace, usage, duration_ms,
       run_by_user_id, run_at, agent_updated_at, created_at, updated_at
FROM   agent_executions
WHERE  conversation_id = $1
  AND  source = 'composer'
ORDER BY run_at DESC, id DESC
LIMIT 1;

-- name: latest-executions-by-conversation
-- For each enabled agent that has at least one composer-source run on
-- this conversation, return the most recent run (DISTINCT ON via the
-- supporting index), the agent's current row (joined), and the outcome
-- row (LEFT JOIN; outcome may be NULL).
-- Excludes 'discarded' outcomes per the locked decision.
-- conversation_id resolved to the int by the caller (UUID → ID lookup
-- happens in the handler).
SELECT
    e.id                   AS "agent_execution.id",
    e.conversation_id      AS "agent_execution.conversation_id",
    e.agent_id             AS "agent_execution.agent_id",
    e.source               AS "agent_execution.source",
    e.resolution_attempt_id AS "agent_execution.resolution_attempt_id",
    e.trace_id             AS "agent_execution.trace_id",
    e.format_version       AS "agent_execution.format_version",
    e.status               AS "agent_execution.status",
    e.draft                AS "agent_execution.draft",
    e.trace                AS "agent_execution.trace",
    e.usage                AS "agent_execution.usage",
    e.duration_ms          AS "agent_execution.duration_ms",
    e.run_by_user_id       AS "agent_execution.run_by_user_id",
    e.run_at               AS "agent_execution.run_at",
    e.agent_updated_at     AS "agent_execution.agent_updated_at",
    e.created_at           AS "agent_execution.created_at",
    e.updated_at           AS "agent_execution.updated_at",
    o.id                   AS "outcome.id",
    o.agent_execution_id   AS "outcome.agent_execution_id",
    o.outcome              AS "outcome.outcome",
    o.outcome_reason       AS "outcome.outcome_reason",
    o.message_id           AS "outcome.message_id",
    o.committed_by_user_id AS "outcome.committed_by_user_id",
    o.committed_at         AS "outcome.committed_at",
    o.created_at           AS "outcome.created_at",
    a.id                   AS "agent.id",
    a.name                 AS "agent.name",
    a.slug                 AS "agent.slug",
    a.enabled              AS "agent.enabled",
    a.updated_at           AS "agent.updated_at"
FROM (
    SELECT DISTINCT ON (agent_id) *
    FROM   agent_executions
    WHERE  conversation_id = $1
      AND  source = 'composer'
    ORDER  BY agent_id, run_at DESC
) e
JOIN      ai_agents a               ON a.id = e.agent_id AND a.enabled = TRUE
LEFT JOIN agent_execution_outcomes o ON o.agent_execution_id = e.id
WHERE     (o.outcome IS NULL OR o.outcome <> 'discarded')
ORDER BY  a.name ASC;

-- name: insert-outcome
-- Constrained by UNIQUE(agent_execution_id). A second insert raises
-- 23505 → handler maps to 409 idempotency response.
INSERT INTO agent_execution_outcomes (
    agent_execution_id, outcome, outcome_reason, message_id, committed_by_user_id
)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, agent_execution_id, outcome, outcome_reason, message_id,
          committed_by_user_id, committed_at, created_at;

-- name: get-outcome-by-execution
SELECT id, agent_execution_id, outcome, outcome_reason, message_id,
       committed_by_user_id, committed_at, created_at
FROM   agent_execution_outcomes
WHERE  agent_execution_id = $1;
