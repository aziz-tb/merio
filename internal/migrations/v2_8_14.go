package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_14 moves the 4 customer-identification executors out of the settings
// table and into the functions table (Unified Executor Framework batch).
// Chunk 1 (v2_8_9) seeded them as customer_identification.executor.* settings
// JSONB rows; those are deleted here and replaced with proper functions rows
// (domain = 'customer_identification'), with empty code + sane defaults that
// operators populate via the admin UI.
//
// Idempotent: DELETE is safe on already-removed rows; INSERT uses ON CONFLICT
// (name) DO NOTHING. Forward-only. Rollback (manual): delete the 4 functions
// rows and re-seed the settings rows (see v2_8_9).
func V2_8_14(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		DELETE FROM settings WHERE key LIKE 'customer_identification.executor.%';

		INSERT INTO functions (name, domain, code, language, packages, timeout_ms, enabled, parameters)
		VALUES
		    ('search_customer_by_fields',    'customer_identification', '', 'python', '', 30000, FALSE, '[]'::jsonb),
		    ('search_contract_by_id',        'customer_identification', '', 'python', '', 30000, FALSE, '[]'::jsonb),
		    ('search_customer_by_id',        'customer_identification', '', 'python', '', 30000, FALSE, '[]'::jsonb),
		    ('search_customer_by_free_text', 'customer_identification', '', 'python', '', 30000, FALSE, '[]'::jsonb)
		ON CONFLICT (name) DO NOTHING;
	`); err != nil {
		return err
	}
	return nil
}
