-- name: get-contract-by-id
SELECT id, created_at, updated_at, last_synced_at,
       contract_id, customer_id, business, firstname, lastname, company, email, phone,
       zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data
FROM   contracts
WHERE  id = $1;

-- name: get-contract-by-contract-id
SELECT id, created_at, updated_at, last_synced_at,
       contract_id, customer_id, business, firstname, lastname, company, email, phone,
       zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data
FROM   contracts
WHERE  contract_id = $1;

-- name: list-contracts-by-customer-id
SELECT id, created_at, updated_at, last_synced_at,
       contract_id, customer_id, business, firstname, lastname, company, email, phone,
       zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data
FROM   contracts
WHERE  customer_id = $1
ORDER  BY id ASC;

-- name: insert-contract
INSERT INTO contracts (
    contract_id, customer_id, business, firstname, lastname, company, email, phone,
    zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, created_at, updated_at, last_synced_at,
          contract_id, customer_id, business, firstname, lastname, company, email, phone,
          zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data;

-- name: upsert-contract
-- Insert a freshly-loaded ERP contract, or refresh the cached row when one
-- already exists for this contract_id. On conflict every stable column is
-- overwritten and last_synced_at is bumped so the TTL clock restarts.
INSERT INTO contracts (
    contract_id, customer_id, business, firstname, lastname, company, email, phone,
    zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data,
    last_synced_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
ON CONFLICT (contract_id) DO UPDATE SET
    customer_id      = EXCLUDED.customer_id,
    business         = EXCLUDED.business,
    firstname        = EXCLUDED.firstname,
    lastname         = EXCLUDED.lastname,
    company          = EXCLUDED.company,
    email            = EXCLUDED.email,
    phone            = EXCLUDED.phone,
    zip              = EXCLUDED.zip,
    delivery_address = EXCLUDED.delivery_address,
    billing_address  = EXCLUDED.billing_address,
    agent_preview    = EXCLUDED.agent_preview,
    erp_urls         = EXCLUDED.erp_urls,
    extra_data       = EXCLUDED.extra_data,
    last_synced_at   = NOW(),
    updated_at       = NOW()
RETURNING id, created_at, updated_at, last_synced_at,
          contract_id, customer_id, business, firstname, lastname, company, email, phone,
          zip, delivery_address, billing_address, agent_preview, erp_urls, extra_data;

-- name: update-contract-last-synced-at
-- Bumps the TTL clock after a fresh ERP fetch that didn't change row data.
UPDATE contracts SET last_synced_at = NOW() WHERE id = $1;

-- name: invalidate-contract-cache
-- Forces the next read to refetch by pushing last_synced_at into the far
-- past (last_synced_at is NOT NULL, so we use epoch rather than NULL).
UPDATE contracts SET last_synced_at = 'epoch'::timestamptz WHERE contract_id = $1;
