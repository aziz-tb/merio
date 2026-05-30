-- name: list-secrets
SELECT id, name, created_at, updated_at, last_accessed_at, created_by_user_id
FROM   secrets
ORDER  BY name ASC;

-- name: insert-secret
INSERT INTO secrets (name, encrypted_value, nonce, created_by_user_id)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: update-secret
UPDATE secrets
SET    encrypted_value = $2,
       nonce           = $3,
       updated_at      = NOW()
WHERE  name = $1;

-- name: delete-secret
DELETE FROM secrets WHERE name = $1;

-- name: get-secret-value-by-name
SELECT id, name, encrypted_value, nonce, created_at, updated_at,
       last_accessed_at, created_by_user_id
FROM   secrets
WHERE  name = $1;

-- name: list-secret-values
SELECT id, name, encrypted_value, nonce, created_at, updated_at,
       last_accessed_at, created_by_user_id
FROM   secrets
ORDER  BY name ASC;

-- name: touch-secret
UPDATE secrets SET last_accessed_at = NOW() WHERE name = $1;

