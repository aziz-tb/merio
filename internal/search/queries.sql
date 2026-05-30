-- name: search-conversations
SELECT
    c.created_at,
    c.uuid,
    c.reference_number,
    c.subject,
    COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.first_name, u.last_name)), ''), '') AS contact_name,
    COALESCE(u.email, '') AS contact_email,
    LEFT(COALESCE(c.last_message, ''), 80) AS last_message_snippet,
    cs.name AS status
FROM conversations c
JOIN users u ON c.contact_id = u.id
LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
WHERE c.reference_number::text ILIKE '%' || $1 || '%'
   OR TRIM(CONCAT_WS(' ', u.first_name, u.last_name)) ILIKE '%' || $1 || '%'
   OR u.email ILIKE '%' || $1 || '%'
ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC
LIMIT 10;

-- name: search-messages
SELECT
    c.created_at as "conversation_created_at",
    c.reference_number as "conversation_reference_number",
    c.uuid as "conversation_uuid",
    m.text_content,
    cs.name as "conversation_status"
FROM conversation_messages m
    JOIN conversations c ON m.conversation_id = c.id
    LEFT JOIN conversation_statuses cs ON c.status_id = cs.id
WHERE m.type != 'activity' and m.text_content ILIKE '%' || $1 || '%'
LIMIT 30;

-- name: search-contacts
SELECT
    id,
    created_at,
    first_name,
    last_name,
    email,
    external_user_id
FROM users
WHERE type = 'contact'
AND deleted_at IS NULL
AND email ILIKE '%' || $1 || '%'
LIMIT 15;
