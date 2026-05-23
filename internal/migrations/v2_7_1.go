package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_1 seeds the ai_providers row used by the F7 Anthropic provider
// (chunk 3 of the F7 batch). v2.7.0 already extended the ai_provider
// enum to include 'anthropic'; this migration adds the row that the
// AnthropicProvider implementation reads its API key from.
//
// Split from v2.7.0 so DBs that already ran v2.7.0 (chunk 1) still pick
// up the row when they upgrade. Idempotent via ON CONFLICT.
//
// Forward-only. To roll back: DELETE FROM ai_providers WHERE provider='anthropic';
// (the v2.7.0 enum extension is harmless to leave in place.)
func V2_7_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		INSERT INTO ai_providers (name, provider, config, is_default)
		VALUES ('anthropic', 'anthropic'::ai_provider, '{"api_key": ""}'::jsonb, FALSE)
		ON CONFLICT DO NOTHING;
	`); err != nil {
		return err
	}
	return nil
}
