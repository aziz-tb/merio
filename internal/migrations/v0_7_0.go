package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_7_0 adds the singleton composer_config table — the one global Composer
// configuration (guidance / automation / escalation), with per-item inbox scope
// (Model A; AI_LAYER_DESIGN.md §7). The id=1 CHECK enforces the singleton; the
// seed row makes GetConfig always resolve on upgraded installs (fresh installs
// get the same row from schema.sql, and GetConfig tolerates its absence
// regardless). Forward-only and idempotent (IF NOT EXISTS + ON CONFLICT).
func V0_7_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS composer_config (
			id          INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			guidance    JSONB NOT NULL DEFAULT '[]'::jsonb,
			automation  JSONB NOT NULL DEFAULT '{"auto_send":false,"approval_required_tool_slugs":[]}'::jsonb,
			escalation  JSONB NOT NULL DEFAULT '{"rules":[],"guidance":[]}'::jsonb,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		INSERT INTO composer_config (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
	`)
	return err
}
