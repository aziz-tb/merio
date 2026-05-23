-- name: get-all
SELECT
    t.id, t.created_at, t.updated_at, t.name, t.parent_id, t.type, t.color,
    t.visibility, t.description, t.detection, t.sla_policy_id, t.priority_id,
    t.updated_by,
    CONCAT_WS(' ', u.first_name, u.last_name) AS updated_by_name
FROM tags t
LEFT JOIN users u ON u.id = t.updated_by
ORDER BY t.name ASC;

-- name: get-public
SELECT
    t.id, t.created_at, t.updated_at, t.name, t.parent_id, t.type, t.color,
    t.visibility, t.description, t.detection, t.sla_policy_id, t.priority_id,
    t.updated_by,
    CONCAT_WS(' ', u.first_name, u.last_name) AS updated_by_name
FROM tags t
LEFT JOIN users u ON u.id = t.updated_by
WHERE t.visibility = 'public'
ORDER BY t.name ASC;

-- name: get-enabled-for-classification
SELECT
    t.id, t.created_at, t.updated_at, t.name, t.parent_id, t.type, t.color,
    t.visibility, t.description, t.detection, t.sla_policy_id, t.priority_id,
    t.updated_by,
    '' AS updated_by_name
FROM tags t
WHERE t.visibility != 'disabled'
  AND COALESCE(t.detection->>'mode', 'manual_only') != 'manual_only'
ORDER BY t.name ASC;

-- name: get
SELECT
    t.id, t.created_at, t.updated_at, t.name, t.parent_id, t.type, t.color,
    t.visibility, t.description, t.detection, t.sla_policy_id, t.priority_id,
    t.updated_by,
    CONCAT_WS(' ', u.first_name, u.last_name) AS updated_by_name
FROM tags t
LEFT JOIN users u ON u.id = t.updated_by
WHERE t.id = $1;

-- name: insert
INSERT INTO tags (
    name, parent_id, type, color, visibility, description, detection,
    sla_policy_id, priority_id, updated_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING
    id, created_at, updated_at, name, parent_id, type, color, visibility,
    description, detection, sla_policy_id, priority_id, updated_by,
    '' AS updated_by_name;

-- name: update
UPDATE tags SET
    name = $2,
    parent_id = $3,
    type = $4,
    color = $5,
    visibility = $6,
    description = $7,
    detection = $8,
    sla_policy_id = $9,
    priority_id = $10,
    updated_by = $11,
    updated_at = NOW()
WHERE id = $1
RETURNING
    id, created_at, updated_at, name, parent_id, type, color, visibility,
    description, detection, sla_policy_id, priority_id, updated_by,
    '' AS updated_by_name;

-- name: delete
DELETE FROM tags WHERE id = $1;

-- name: bulk-import
INSERT INTO tags (name) VALUES ($1)
ON CONFLICT (name) DO NOTHING;
