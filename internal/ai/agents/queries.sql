-- name: get-all
SELECT id, created_at, updated_at, name, slug, description, prompt,
       detection, tools, kb_scope, preset_overrides,
       max_iterations, extended_thinking_enabled, min_verification, enabled
FROM   ai_agents
WHERE  ($1::BOOL IS NULL OR enabled = $1)
ORDER  BY id ASC
LIMIT  $2 OFFSET $3;

-- name: count-all
SELECT COUNT(*)::INT
FROM   ai_agents
WHERE  ($1::BOOL IS NULL OR enabled = $1);

-- name: get-by-id
SELECT id, created_at, updated_at, name, slug, description, prompt,
       detection, tools, kb_scope, preset_overrides,
       max_iterations, extended_thinking_enabled, min_verification, enabled
FROM   ai_agents
WHERE  id = $1;

-- name: get-by-slug
SELECT id, created_at, updated_at, name, slug, description, prompt,
       detection, tools, kb_scope, preset_overrides,
       max_iterations, extended_thinking_enabled, min_verification, enabled
FROM   ai_agents
WHERE  slug = $1;

-- name: insert
INSERT INTO ai_agents (
    name, slug, description, prompt, detection, tools, kb_scope,
    preset_overrides, max_iterations, extended_thinking_enabled, min_verification,
    enabled
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, created_at, updated_at, name, slug, description, prompt,
          detection, tools, kb_scope, preset_overrides,
          max_iterations, extended_thinking_enabled, min_verification, enabled;

-- name: update
UPDATE ai_agents
SET    name                 = $2,
       slug                 = $3,
       description          = $4,
       prompt               = $5,
       detection            = $6,
       tools                = $7,
       kb_scope             = $8,
       preset_overrides     = $9,
       max_iterations       = $10,
       extended_thinking_enabled = $11,
       min_verification     = $12,
       enabled              = $13,
       updated_at           = NOW()
WHERE  id = $1
RETURNING id, created_at, updated_at, name, slug, description, prompt,
          detection, tools, kb_scope, preset_overrides,
          max_iterations, extended_thinking_enabled, min_verification, enabled;

-- name: delete
DELETE FROM ai_agents WHERE id = $1;
