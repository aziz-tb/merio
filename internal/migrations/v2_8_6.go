package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_6 lands the customer / contract identification data model
// (Wave 3 Batch 10, design Sections 3.1, 3.2, 3.4):
//
//   - customers: a cache of ERP-resolved customer records. Stable fields
//     are typed columns; everything else lives in extra_data JSONB.
//     last_synced_at drives the 60-minute TTL.
//   - contracts: same shape, plus contract-specific columns (zip,
//     agent_preview, erp_urls) and an FK to customers(id) ON DELETE
//     CASCADE.
//   - conversations gains four nullable identification columns
//     (customer_id, contract_id, legitimation_level, legitimation_message)
//     with FKs ON DELETE SET NULL so deleting a cached customer/contract
//     never deletes a conversation.
//
// DDL is copied verbatim from the design doc. The two CHECK constraints
// enforce the business/individual branching (a company row has company
// set; an individual row has lastname set). Partial indexes match only
// the rows worth indexing (non-null email/phone/zip/customer_id/
// contract_id).
//
// Idempotent via CREATE TABLE / INDEX IF NOT EXISTS and ADD COLUMN IF
// NOT EXISTS. Forward-only. Rollback (manual):
//
//	ALTER TABLE conversations
//	    DROP COLUMN IF EXISTS customer_id,
//	    DROP COLUMN IF EXISTS contract_id,
//	    DROP COLUMN IF EXISTS legitimation_level,
//	    DROP COLUMN IF EXISTS legitimation_message;
//	DROP TABLE IF EXISTS contracts;
//	DROP TABLE IF EXISTS customers;
func V2_8_6(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. customers (design Section 3.1).
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS customers (
		    id              BIGSERIAL PRIMARY KEY,
		    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

		    external_id     TEXT NOT NULL,
		    business        BOOLEAN NOT NULL,

		    firstname       TEXT NULL,
		    lastname        TEXT NULL,
		    company         TEXT NULL,
		    email           TEXT NULL,
		    phone           TEXT NULL,
		    delivery_address TEXT NULL,
		    billing_address TEXT NULL,

		    extra_data      JSONB NOT NULL DEFAULT '{}'::jsonb,

		    CONSTRAINT constraint_customers_on_external_id_unique
		        UNIQUE (external_id),
		    CONSTRAINT constraint_customers_on_business_consistency
		        CHECK (
		            (business = TRUE  AND company IS NOT NULL) OR
		            (business = FALSE AND lastname IS NOT NULL)
		        )
		);
		CREATE INDEX IF NOT EXISTS index_customers_on_email ON customers (email) WHERE email IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_customers_on_phone ON customers (phone) WHERE phone IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_customers_on_last_synced_at ON customers (last_synced_at);
	`); err != nil {
		return err
	}

	// 2. contracts (design Section 3.2).
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS contracts (
		    id              BIGSERIAL PRIMARY KEY,
		    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

		    external_id     TEXT NOT NULL,
		    customer_id     BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
		    business        BOOLEAN NOT NULL,

		    firstname       TEXT NULL,
		    lastname        TEXT NULL,
		    company         TEXT NULL,
		    email           TEXT NULL,
		    phone           TEXT NULL,
		    zip             TEXT NULL,
		    delivery_address TEXT NULL,
		    billing_address TEXT NULL,

		    agent_preview   JSONB NOT NULL DEFAULT '[]'::jsonb,
		    erp_urls        JSONB NOT NULL DEFAULT '[]'::jsonb,
		    extra_data      JSONB NOT NULL DEFAULT '{}'::jsonb,

		    CONSTRAINT constraint_contracts_on_external_id_unique
		        UNIQUE (external_id),
		    CONSTRAINT constraint_contracts_on_business_consistency
		        CHECK (
		            (business = TRUE  AND company IS NOT NULL) OR
		            (business = FALSE AND lastname IS NOT NULL)
		        )
		);
		CREATE INDEX IF NOT EXISTS index_contracts_on_customer_id ON contracts (customer_id);
		CREATE INDEX IF NOT EXISTS index_contracts_on_email ON contracts (email) WHERE email IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_contracts_on_phone ON contracts (phone) WHERE phone IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_contracts_on_zip ON contracts (zip) WHERE zip IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_contracts_on_last_synced_at ON contracts (last_synced_at);
	`); err != nil {
		return err
	}

	// 3. conversations identification columns + indexes (design Section 3.4).
	if _, err := db.Exec(`
		ALTER TABLE conversations
		    ADD COLUMN IF NOT EXISTS customer_id          BIGINT   NULL REFERENCES customers(id) ON DELETE SET NULL,
		    ADD COLUMN IF NOT EXISTS contract_id          BIGINT   NULL REFERENCES contracts(id) ON DELETE SET NULL,
		    ADD COLUMN IF NOT EXISTS legitimation_level   SMALLINT NOT NULL DEFAULT 0,
		    ADD COLUMN IF NOT EXISTS legitimation_message TEXT     NULL;

		CREATE INDEX IF NOT EXISTS index_conversations_on_customer_id
		    ON conversations (customer_id) WHERE customer_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_conversations_on_contract_id
		    ON conversations (contract_id) WHERE contract_id IS NOT NULL;
	`); err != nil {
		return err
	}

	return nil
}
