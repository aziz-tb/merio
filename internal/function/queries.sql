-- name: get-function-by-name
SELECT id, created_at, updated_at, name, domain, code, language, packages,
       timeout_ms, parameters, updated_by_user_id
FROM   functions
WHERE  name = $1;

-- name: get-function-by-id
SELECT id, created_at, updated_at, name, domain, code, language, packages,
       timeout_ms, parameters, updated_by_user_id
FROM   functions
WHERE  id = $1;

-- name: list-functions-by-domain
SELECT id, created_at, updated_at, name, domain, code, language, packages,
       timeout_ms, parameters, updated_by_user_id
FROM   functions
WHERE  ($1::TEXT = '' OR domain = $1)
ORDER  BY name ASC;

-- name: insert-function
INSERT INTO functions (
    name, domain, code, language, packages, timeout_ms, parameters, updated_by_user_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: update-function-by-name
-- Updates the mutable fields only; name and domain are immutable.
UPDATE functions
SET    code               = $2,
       language           = $3,
       packages           = $4,
       timeout_ms         = $5,
       parameters         = $6,
       updated_by_user_id = $7,
       updated_at         = NOW()
WHERE  name = $1;

-- name: delete-function-by-name
DELETE FROM functions WHERE name = $1;
