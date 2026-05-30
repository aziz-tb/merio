package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_20 allows ai_operations to represent in-flight workflow state. The
// customer-identification sidebar reads this to keep its spinner alive across
// page reloads.
func V2_8_20(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE ai_operations
			DROP CONSTRAINT IF EXISTS check_ai_operations_status;

		ALTER TABLE ai_operations
			ADD CONSTRAINT check_ai_operations_status
			CHECK (status IN ('running', 'success', 'failed', 'skipped'));
	`)
	return err
}
