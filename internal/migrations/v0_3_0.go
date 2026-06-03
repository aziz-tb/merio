package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_3_0 adds an explicit marker for non-human system actors.
func V0_3_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE users
			ADD COLUMN IF NOT EXISTS is_system BOOL NOT NULL DEFAULT FALSE;
	`)
	return err
}
