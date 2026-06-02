-- name: get-customer-by-id
SELECT id, created_at, updated_at, last_synced_at,
       customer_id, business, firstname, lastname, company, email, phone,
       zip, delivery_address, billing_address, contract_ids, extra_data
FROM   customers
WHERE  id = $1;

-- name: get-customer-by-customer-id
SELECT id, created_at, updated_at, last_synced_at,
       customer_id, business, firstname, lastname, company, email, phone,
       zip, delivery_address, billing_address, contract_ids, extra_data
FROM   customers
WHERE  customer_id = $1;

-- name: insert-customer
INSERT INTO customers (
    customer_id, business, firstname, lastname, company, email, phone,
    zip, delivery_address, billing_address, contract_ids, extra_data
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, created_at, updated_at, last_synced_at,
          customer_id, business, firstname, lastname, company, email, phone,
          zip, delivery_address, billing_address, contract_ids, extra_data;

-- name: upsert-customer
-- Insert a freshly-loaded ERP customer, or refresh the cached row when one
-- already exists for this customer_id. On conflict every stable column is
-- overwritten and last_synced_at is bumped so the TTL clock restarts.
INSERT INTO customers (
    customer_id, business, firstname, lastname, company, email, phone,
    zip, delivery_address, billing_address, contract_ids, extra_data, last_synced_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
ON CONFLICT (customer_id) DO UPDATE SET
    business         = EXCLUDED.business,
    firstname        = EXCLUDED.firstname,
    lastname         = EXCLUDED.lastname,
    company          = EXCLUDED.company,
    email            = EXCLUDED.email,
    phone            = EXCLUDED.phone,
    zip              = EXCLUDED.zip,
    delivery_address = EXCLUDED.delivery_address,
    billing_address  = EXCLUDED.billing_address,
    contract_ids     = EXCLUDED.contract_ids,
    extra_data       = EXCLUDED.extra_data,
    last_synced_at   = NOW(),
    updated_at       = NOW()
RETURNING id, created_at, updated_at, last_synced_at,
          customer_id, business, firstname, lastname, company, email, phone,
          zip, delivery_address, billing_address, contract_ids, extra_data;

-- name: update-customer-last-synced-at
-- Bumps the TTL clock after a fresh ERP fetch that didn't change row data.
UPDATE customers SET last_synced_at = NOW() WHERE id = $1;

-- name: invalidate-customer-cache
-- Forces the next read to refetch by pushing last_synced_at into the far
-- past (last_synced_at is NOT NULL, so we use epoch rather than NULL).
UPDATE customers SET last_synced_at = 'epoch'::timestamptz WHERE customer_id = $1;
