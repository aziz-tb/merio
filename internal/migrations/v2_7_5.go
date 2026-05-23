package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_5 reshapes ai_agents knowledge-base scoping and adds a per-agent
// legitimation floor, as part of the Wave-2 agent/tool UI redesign:
//
//   - kb_scope JSONB ({"folder_ids":[…], "article_ids":[…]}) replaces
//     kb_folder_ids BIGINT[]. The new shape lets an admin whitelist
//     individual articles in addition to whole folders — required by the
//     KB tree picker that ships with the redesigned agent edit form.
//   - min_legitimation INT (NOT NULL DEFAULT 0) becomes the agent's
//     minimum trust level. The tool resolver already filters by tool
//     min_legitimation; this field lets a high-trust agent refuse to
//     even start on a low-legitimation conversation. Mirrors the column
//     on ai_tools so admins reason in the same vocabulary.
//
// Backfill rule: every non-empty kb_folder_ids row becomes
//   {"folder_ids": <the array>, "article_ids": []}
// Empty / NULL kb_folder_ids rows keep the column default
//   {"folder_ids": [], "article_ids": []}
//
// Idempotent via ADD COLUMN IF NOT EXISTS / DROP COLUMN IF EXISTS and
// the WHERE-kb_folder_ids-still-exists guard on the backfill UPDATE
// (executed only after we re-check the column exists).
//
// Forward-only. Rollback (manual):
//
//	BEGIN;
//	ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS kb_folder_ids BIGINT[] NOT NULL DEFAULT '{}';
//	UPDATE ai_agents SET kb_folder_ids =
//	    ARRAY(SELECT jsonb_array_elements_text(kb_scope->'folder_ids')::BIGINT);
//	ALTER TABLE ai_agents DROP COLUMN IF EXISTS kb_scope;
//	ALTER TABLE ai_agents DROP COLUMN IF EXISTS min_legitimation;
//	COMMIT;
func V2_7_5(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Add kb_scope with the empty default. NOT NULL is safe because the
	//    column has a default; existing rows materialize as
	//    {"folder_ids":[],"article_ids":[]} before the backfill rewrites them.
	if _, err := db.Exec(`
		ALTER TABLE ai_agents
		ADD COLUMN IF NOT EXISTS kb_scope JSONB NOT NULL
		    DEFAULT '{"folder_ids":[],"article_ids":[]}'::jsonb;
	`); err != nil {
		return err
	}

	// 2. Add min_legitimation with a sane default. The CHECK matches the
	//    style on ai_tools.min_legitimation so future legitimation_level
	//    changes only touch one constraint shape.
	if _, err := db.Exec(`
		ALTER TABLE ai_agents
		ADD COLUMN IF NOT EXISTS min_legitimation INT NOT NULL DEFAULT 0;
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		ALTER TABLE ai_agents
		DROP CONSTRAINT IF EXISTS constraint_ai_agents_on_min_legitimation;
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		ALTER TABLE ai_agents
		ADD CONSTRAINT constraint_ai_agents_on_min_legitimation
		    CHECK (min_legitimation >= 0);
	`); err != nil {
		return err
	}

	// 3. Backfill kb_scope from kb_folder_ids when the legacy column still
	//    exists. The `to_regclass` + information_schema guard makes the
	//    UPDATE a no-op on a DB that already dropped the column (idempotent
	//    re-run after a successful migration).
	if _, err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE  table_name = 'ai_agents'
				  AND  column_name = 'kb_folder_ids'
			) THEN
				UPDATE ai_agents
				SET    kb_scope = jsonb_build_object(
				           'folder_ids', to_jsonb(kb_folder_ids),
				           'article_ids', '[]'::jsonb
				       )
				WHERE  kb_folder_ids IS NOT NULL
				  AND  array_length(kb_folder_ids, 1) > 0;
			END IF;
		END $$;
	`); err != nil {
		return err
	}

	// 4. Drop the legacy column.
	if _, err := db.Exec(`
		ALTER TABLE ai_agents DROP COLUMN IF EXISTS kb_folder_ids;
	`); err != nil {
		return err
	}

	return nil
}
