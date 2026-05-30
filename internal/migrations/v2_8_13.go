package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_13 repoints ai_tools at the functions table (Unified Executor
// Framework batch). The code/language/packages that used to live in
// ai_tools.implementation JSONB now live in a functions row; ai_tools keeps
// only AI-facing metadata (LLM schema, category, agent links) plus a
// function_id FK to its code.
//
// This is dev — no production data. The migration TRUNCATEs ai_tools rather
// than backfilling; operators recreate their test tools through the admin
// UI afterwards, which now transparently writes both a functions row and an
// ai_tools row. Nothing FK-references ai_tools (only an index), so the
// CASCADE reach is contained to ai_tools itself.
//
// The TRUNCATE is guarded on the implementation column still existing, so a
// manual re-run after operators have created real tools does NOT wipe them
// (on a migrated DB the column is already gone → guard is false → no-op).
// DROP/ADD use IF EXISTS / IF NOT EXISTS for re-run safety.
//
// Forward-only. Rollback (manual):
//
//	ALTER TABLE ai_tools DROP COLUMN IF EXISTS function_id;
//	ALTER TABLE ai_tools ADD COLUMN IF NOT EXISTS implementation JSONB NOT NULL DEFAULT '{}'::jsonb;
func V2_8_13(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_name = 'ai_tools' AND column_name = 'implementation') THEN
				TRUNCATE ai_tools CASCADE;
			END IF;
		END $$;

		ALTER TABLE ai_tools DROP COLUMN IF EXISTS implementation;
		ALTER TABLE ai_tools
		    ADD COLUMN IF NOT EXISTS function_id BIGINT NOT NULL REFERENCES functions(id) ON DELETE RESTRICT;
	`); err != nil {
		return err
	}
	return nil
}
