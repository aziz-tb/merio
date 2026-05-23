package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_0 prepares the database for the F7 LLM provider abstraction and the
// F5 tool registry / agent runner build (Wave 2). This migration is the
// "schema-only" first step:
//
//   - extends the ai_provider enum to include 'anthropic' so the Anthropic
//     provider (chunk 3) has a row to encrypt its key into via the existing
//     ai_providers table pattern
//   - drops ai_agents.model_preset (replaced by file-based presets in
//     config/ai_presets.json — see chunk-2 preset registry)
//   - adds ai_agents.preset_overrides JSONB to let an agent override the
//     default purpose→preset mapping per-purpose
//
// File-based presets are a deliberate v1 choice: presets are platform-tuning
// decisions, not tenant config; changes belong in version control via PR,
// not live DB edits. No ai_presets / ai_purpose_presets tables exist or
// will be created by this migration.
//
// Idempotent. Forward-only — to roll back, run the SQL below manually
// against the same database, then redeploy a build that no longer references
// the new column:
//
//	BEGIN;
//	ALTER TABLE ai_agents DROP COLUMN IF EXISTS preset_overrides;
//	ALTER TABLE ai_agents ADD COLUMN model_preset TEXT NOT NULL DEFAULT 'default';
//	-- Postgres doesn't support removing enum values; the 'anthropic' value
//	-- stays in ai_provider, harmlessly. Drop the type and recreate if
//	-- absolutely necessary (will require dropping ai_providers rows first).
//	COMMIT;
func V2_7_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Extend the ai_provider enum. ADD VALUE IF NOT EXISTS is idempotent;
	//    cannot be wrapped inside an explicit transaction in older Postgres,
	//    but our migration runner uses autocommit per Exec which works.
	if _, err := db.Exec(`
		ALTER TYPE ai_provider ADD VALUE IF NOT EXISTS 'anthropic';
	`); err != nil {
		return err
	}

	// 2. ai_agents column swap. Drop the now-obsolete model_preset; add the
	//    preset_overrides JSONB. Both DDL ops are idempotent (DROP IF EXISTS
	//    / ADD IF NOT EXISTS). The seeded test agent's existing
	//    model_preset='default' value is simply discarded with the column.
	if _, err := db.Exec(`
		ALTER TABLE ai_agents DROP COLUMN IF EXISTS model_preset;
		ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS preset_overrides
			JSONB NOT NULL DEFAULT '{}'::jsonb;
	`); err != nil {
		return err
	}

	return nil
}
