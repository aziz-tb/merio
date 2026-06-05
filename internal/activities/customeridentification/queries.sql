-- name: get-message-for-identification
SELECT
    m.id,
    m.conversation_id,
    m.type,
    m.sender_type,
    m.private,
    m.content,
    m.text_content,
    m.content_type,
    COALESCE(c.subject, '') AS subject,
    COALESCE(u.email, '') AS sender_email
FROM conversation_messages m
JOIN conversations c ON c.id = m.conversation_id
JOIN users u ON u.id = m.sender_id
WHERE m.id = $1 AND m.conversation_id = $2;

-- name: find-fresh-customer-by-email
SELECT customer_id
FROM customers
WHERE lower(email) = lower($1)
  AND last_synced_at >= NOW() - ($2::int * INTERVAL '1 minute')
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: find-fresh-contract-by-email
SELECT contract_id, customer_id
FROM contracts
WHERE lower(email) = lower($1)
  AND last_synced_at >= NOW() - ($2::int * INTERVAL '1 minute')
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: get-conversation-identification-links
SELECT
    uuid,
    contact_id,
    customer_id,
    contract_id,
    verification_level,
    (
        SELECT linked_customer_id
        FROM users
        WHERE users.id = conversations.contact_id
    ) AS linked_customer_id
FROM conversations
WHERE id = $1;

-- name: update-conversation-identification
UPDATE conversations
SET customer_id = $2,
    contract_id = COALESCE(
        $3,
        (
            SELECT c.contract_id
            FROM contracts c
            WHERE c.customer_id = $2
            ORDER BY c.created_at ASC
            LIMIT 1
        )
    ),
    verification_level = $4,
    verification_message = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: update-contact-linked-customer
UPDATE users
SET linked_customer_id = $2,
    linked_customer_id_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: insert-ai-operation
INSERT INTO ai_operations (
    operation_type, conversation_id, triggered_by, triggered_by_user_id,
    trace_id, status, error_message, payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: update-ai-operation
UPDATE ai_operations
SET status = $2,
    error_message = $3,
    payload = $4
WHERE id = $1;
