package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_27 adds the Urgent conversation priority.
func V2_8_27(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		INSERT INTO conversation_priorities (name)
		VALUES ('Urgent')
		ON CONFLICT (name) DO NOTHING;
	`)
	return err
}
