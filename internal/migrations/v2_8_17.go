package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_17 aligns ai_tools.timeout_ms with the linked functions.timeout_ms
// constraint. Function execution is the binding runtime cap, so the
// metadata row must accept the same maximum.
func V2_8_17(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE ai_tools
		    DROP CONSTRAINT IF EXISTS constraint_ai_tools_on_timeout;
		ALTER TABLE ai_tools
		    ADD CONSTRAINT constraint_ai_tools_on_timeout
		    CHECK (timeout_ms > 0 AND timeout_ms <= 120000);
	`)
	return err
}
