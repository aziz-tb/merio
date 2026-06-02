package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_18 drops the vestigial top-level custom criteria setting. Custom
// criteria now live inside each legitimation rule group under
// customer_identification.legitimation_rules_async.
func V2_8_18(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		DELETE FROM settings
		WHERE key = 'customer_identification.custom_criteria';
	`)
	return err
}
