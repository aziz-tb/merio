package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_1 adds a per-agent extended-thinking toggle. Existing agents
// default to enabled so Batch 4 behavior is preserved unless an
// operator explicitly disables it.
func V2_8_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE ai_agents
		ADD COLUMN IF NOT EXISTS extended_thinking_enabled BOOL NOT NULL DEFAULT TRUE;
	`)
	return err
}
