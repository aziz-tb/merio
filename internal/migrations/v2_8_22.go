package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_22 promotes ERP customer/contract ids to the cross-table identifiers.
//
// BIGSERIAL id columns remain internal row identifiers, but contracts and
// conversations now reference customers(customer_id) and contracts(contract_id)
// directly. The operator purges customer/contract cache data before applying
// this dev migration, so no BIGINT FK values are preserved or converted here.
func V2_8_22(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_customer_id_fkey;
		ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_contract_id_fkey;
		ALTER TABLE contracts DROP CONSTRAINT IF EXISTS contracts_customer_id_fkey;

		DROP INDEX IF EXISTS index_contracts_on_customer_id;
		DROP INDEX IF EXISTS index_conversations_on_customer_id;
		DROP INDEX IF EXISTS index_conversations_on_contract_id;

		ALTER TABLE conversations DROP COLUMN IF EXISTS customer_id;
		ALTER TABLE conversations DROP COLUMN IF EXISTS contract_id;
		ALTER TABLE contracts DROP COLUMN IF EXISTS customer_id;

		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'customers' AND column_name = 'external_id'
			) THEN
				ALTER TABLE customers RENAME COLUMN external_id TO customer_id;
			END IF;
		END $$;

		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'contracts' AND column_name = 'external_id'
			) THEN
				ALTER TABLE contracts RENAME COLUMN external_id TO contract_id;
			END IF;
		END $$;

		ALTER TABLE customers DROP CONSTRAINT IF EXISTS constraint_customers_on_external_id_unique;
		ALTER TABLE contracts DROP CONSTRAINT IF EXISTS constraint_contracts_on_external_id_unique;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'constraint_customers_on_customer_id_unique'
			) THEN
				ALTER TABLE customers
					ADD CONSTRAINT constraint_customers_on_customer_id_unique UNIQUE (customer_id);
			END IF;
		END $$;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'constraint_contracts_on_contract_id_unique'
			) THEN
				ALTER TABLE contracts
					ADD CONSTRAINT constraint_contracts_on_contract_id_unique UNIQUE (contract_id);
			END IF;
		END $$;

		ALTER TABLE contracts
			ADD COLUMN IF NOT EXISTS customer_id TEXT NOT NULL
				REFERENCES customers(customer_id) ON UPDATE CASCADE ON DELETE CASCADE;

		ALTER TABLE conversations
			ADD COLUMN IF NOT EXISTS customer_id TEXT
				REFERENCES customers(customer_id) ON UPDATE CASCADE ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS contract_id TEXT
				REFERENCES contracts(contract_id) ON UPDATE CASCADE ON DELETE SET NULL;

		CREATE INDEX IF NOT EXISTS index_contracts_on_customer_id
			ON contracts (customer_id);
		CREATE INDEX IF NOT EXISTS index_conversations_on_customer_id
			ON conversations (customer_id) WHERE customer_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_conversations_on_contract_id
			ON conversations (contract_id) WHERE contract_id IS NOT NULL;
	`)
	return err
}
