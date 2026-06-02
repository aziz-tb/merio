package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_28 establishes Zone 1/Zone 2 customer and contract storage.
func V2_8_28(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE customers ADD COLUMN IF NOT EXISTS zip TEXT NULL;
		ALTER TABLE customers ADD COLUMN IF NOT EXISTS contract_ids JSON NOT NULL DEFAULT '[]'::json;
		CREATE INDEX IF NOT EXISTS index_customers_on_zip ON customers (zip) WHERE zip IS NOT NULL;
		ALTER TABLE customers ALTER COLUMN extra_data TYPE JSON USING extra_data::json;

		ALTER TABLE contracts ALTER COLUMN extra_data TYPE JSON USING extra_data::json;

		UPDATE customers SET last_synced_at = TIMESTAMPTZ 'epoch';
		UPDATE contracts SET last_synced_at = TIMESTAMPTZ 'epoch';
	`)
	return err
}
