-- name: get-all
SELECT t.id, t.created_at, t.updated_at, t.name, t.slug, t.description,
       t.input_parameters, t.output_schema, t.category, t.timeout_ms, t.async, t.enabled,
       t.function_id, f.code AS fn_code, f.language AS fn_language, f.packages AS fn_packages
FROM   ai_tools t
JOIN   functions f ON f.id = t.function_id
WHERE  ($1::BOOL IS NULL OR t.enabled = $1)
ORDER  BY t.id ASC
LIMIT  $2 OFFSET $3;

-- name: count-all
SELECT COUNT(*)::INT
FROM   ai_tools
WHERE  ($1::BOOL IS NULL OR enabled = $1);

-- name: get-by-id
SELECT t.id, t.created_at, t.updated_at, t.name, t.slug, t.description,
       t.input_parameters, t.output_schema, t.category, t.timeout_ms, t.async, t.enabled,
       t.function_id, f.code AS fn_code, f.language AS fn_language, f.packages AS fn_packages
FROM   ai_tools t
JOIN   functions f ON f.id = t.function_id
WHERE  t.id = $1;

-- name: get-by-slug
SELECT t.id, t.created_at, t.updated_at, t.name, t.slug, t.description,
       t.input_parameters, t.output_schema, t.category, t.timeout_ms, t.async, t.enabled,
       t.function_id, f.code AS fn_code, f.language AS fn_language, f.packages AS fn_packages
FROM   ai_tools t
JOIN   functions f ON f.id = t.function_id
WHERE  t.slug = $1;

-- name: get-by-ids
SELECT t.id, t.created_at, t.updated_at, t.name, t.slug, t.description,
       t.input_parameters, t.output_schema, t.category, t.timeout_ms, t.async, t.enabled,
       t.function_id, f.code AS fn_code, f.language AS fn_language, f.packages AS fn_packages
FROM   ai_tools t
JOIN   functions f ON f.id = t.function_id
WHERE  t.id = ANY($1::INT[]);

-- name: insert
INSERT INTO ai_tools (
    name, slug, description,
    input_parameters, output_schema, category,
    timeout_ms, async, enabled, function_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, created_at, updated_at, name, slug, description,
          input_parameters, output_schema, category,
          timeout_ms, async, enabled, function_id;

-- name: update
UPDATE ai_tools
SET    name             = $2,
       slug             = $3,
       description      = $4,
       input_parameters = $5,
       output_schema    = $6,
       category         = $7,
       timeout_ms       = $8,
       async            = $9,
       enabled          = $10,
       function_id      = $11,
       updated_at       = NOW()
WHERE  id = $1
RETURNING id, created_at, updated_at, name, slug, description,
          input_parameters, output_schema, category,
          timeout_ms, async, enabled, function_id;

-- name: delete
DELETE FROM ai_tools WHERE id = $1;
