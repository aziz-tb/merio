-- name: get-config
SELECT id, guidance, automation, escalation, created_at, updated_at
FROM   composer_config
WHERE  id = 1;

-- name: upsert
INSERT INTO composer_config (id, guidance, automation, escalation, updated_at)
VALUES (1, $1, $2, $3, NOW())
ON CONFLICT (id) DO UPDATE
SET    guidance   = EXCLUDED.guidance,
       automation = EXCLUDED.automation,
       escalation = EXCLUDED.escalation,
       updated_at = NOW()
RETURNING id, guidance, automation, escalation, created_at, updated_at;
