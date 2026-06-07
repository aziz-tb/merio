package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_5_0 drops the legacy AI provider table. Runtime LLM credentials now live
// in the secrets table / environment and reply-box rewrite only keeps ai_prompts.
func V0_5_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		DROP TABLE IF EXISTS ai_providers CASCADE;
		DROP TYPE IF EXISTS ai_provider;
	`)
	return err
}
