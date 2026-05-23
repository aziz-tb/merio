-- name: get-all
SELECT id, created_at, updated_at, name, slug, description,
       input_parameters, output_schema, implementation, category, min_legitimation,
       timeout_ms, async, enabled
FROM   ai_tools
WHERE  ($1::BOOL IS NULL OR enabled = $1)
ORDER  BY id ASC
LIMIT  $2 OFFSET $3;

-- name: count-all
SELECT COUNT(*)::INT
FROM   ai_tools
WHERE  ($1::BOOL IS NULL OR enabled = $1);

-- name: get-by-id
SELECT id, created_at, updated_at, name, slug, description,
       input_parameters, output_schema, implementation, category, min_legitimation,
       timeout_ms, async, enabled
FROM   ai_tools
WHERE  id = $1;

-- name: get-by-slug
SELECT id, created_at, updated_at, name, slug, description,
       input_parameters, output_schema, implementation, category, min_legitimation,
       timeout_ms, async, enabled
FROM   ai_tools
WHERE  slug = $1;

-- name: get-by-ids
SELECT id, created_at, updated_at, name, slug, description,
       input_parameters, output_schema, implementation, category, min_legitimation,
       timeout_ms, async, enabled
FROM   ai_tools
WHERE  id = ANY($1::INT[]);

-- name: insert
INSERT INTO ai_tools (
    name, slug, description,
    input_parameters, output_schema, implementation, category,
    min_legitimation, timeout_ms, async, enabled
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at, updated_at, name, slug, description,
          input_parameters, output_schema, implementation, category, min_legitimation,
          timeout_ms, async, enabled;

-- name: update
UPDATE ai_tools
SET    name             = $2,
       slug             = $3,
       description      = $4,
       input_parameters = $5,
       output_schema    = $6,
       implementation   = $7,
       category         = $8,
       min_legitimation = $9,
       timeout_ms       = $10,
       async            = $11,
       enabled          = $12,
       updated_at       = NOW()
WHERE  id = $1
RETURNING id, created_at, updated_at, name, slug, description,
          input_parameters, output_schema, implementation, category, min_legitimation,
          timeout_ms, async, enabled;

-- name: delete
DELETE FROM ai_tools WHERE id = $1;
