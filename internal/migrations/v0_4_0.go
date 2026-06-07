package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_4_0 allows env-style uppercase names in the operator secrets table.
func V0_4_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE secrets
			DROP CONSTRAINT IF EXISTS secrets_name_format,
			ADD CONSTRAINT secrets_name_format
				CHECK (name ~ '^[A-Za-z][A-Za-z0-9_]{0,63}$');
	`)
	return err
}
