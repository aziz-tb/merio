package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_2 is the F5 schema-cleanup step (chunk 6 of the F7+F5 batch):
//
//   - renames script_executions → tool_executions. The table is the
//     single audit surface for all tool calls (custom Python tools today,
//     internal capabilities once chunk 10's dispatcher unification lands).
//     RENAME TO preserves all rows and indexes.
//
//   - drops ai_tools.type and ai_tools.category. Only Python custom tools
//     live in ai_tools now; internal capabilities (kb_search,
//     get_customer_data, get_conversation_history) are registered in Go
//     code via internal/ai/tools/builtin/. The category dimension is
//     deferred until tool policy gating actually needs it.
//
//   - deletes the seeded "kb-search-universal" row that F9 chunk 7
//     inserted. That row's whole reason was type=go_builtin; dropping
//     the column makes it meaningless. The kb_search capability lives in
//     code (chunk 8) and replaces it. Per the F9 retrospective no
//     ai_agents.tools entry references this slug, but a defensive check
//     surfaces it loudly if a hand-edited install does.
//
// Idempotent. Forward-only. To roll back, rename in the opposite
// direction and add the columns back; existing audit rows survive.
func V2_7_2(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. script_executions → tool_executions. Postgres preserves data
	//    and existing indexes through RENAME TO. The original index
	//    names keep their `idx_script_executions_*` prefix (Postgres
	//    doesn't auto-rename indexes); we rename them explicitly so
	//    DBA-side `\d tool_executions` doesn't expose stale names.
	//
	//    The IF EXISTS / DO blocks make the migration safe to run on a
	//    fresh schema (where script_executions never existed) and on a
	//    DB that already ran it (where the rename is a no-op).
	if _, err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables
			           WHERE table_schema = current_schema() AND table_name = 'script_executions') THEN
				ALTER TABLE script_executions RENAME TO tool_executions;
			END IF;
		END $$;
	`); err != nil {
		return err
	}
	for _, idx := range []struct {
		oldName string
		newName string
	}{
		{"idx_script_executions_tool_name", "idx_tool_executions_tool_name"},
		{"idx_script_executions_conversation_id", "idx_tool_executions_conversation_id"},
		{"idx_script_executions_started_at", "idx_tool_executions_started_at"},
		{"idx_script_executions_invoker_user_id", "idx_tool_executions_invoker_user_id"},
	} {
		if _, err := db.Exec(`ALTER INDEX IF EXISTS ` + idx.oldName + ` RENAME TO ` + idx.newName + `;`); err != nil {
			return err
		}
	}

	// 2. Drop the F9 seeded "kb-search-universal" row before the
	//    columns it depends on go away. Defensive guard: refuse to
	//    delete it if any agent's tools array references it (would
	//    leave dangling refs).
	if _, err := db.Exec(`
		DO $$
		DECLARE
			refs INT;
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM ai_tools WHERE slug = 'kb-search-universal') THEN
				RETURN;
			END IF;
			SELECT COUNT(*)::INT INTO refs
			FROM   ai_agents
			WHERE  tools::text LIKE '%"kb-search-universal"%'
			   OR  tools::text LIKE '%"kb_search_universal"%';
			IF refs > 0 THEN
				RAISE EXCEPTION 'cannot delete seeded ai_tools row "kb-search-universal": % agent(s) reference it; remove the references first', refs;
			END IF;
			DELETE FROM ai_tools WHERE slug = 'kb-search-universal';
		END $$;
	`); err != nil {
		return err
	}

	// 3. Drop the type-specific index, then the columns themselves.
	//    F9 had `CREATE INDEX index_ai_tools_on_type ON ai_tools(type)`;
	//    Postgres cascades that on DROP COLUMN, but explicit DROP keeps
	//    the order obvious.
	if _, err := db.Exec(`
		DROP INDEX IF EXISTS index_ai_tools_on_type;
		ALTER TABLE ai_tools DROP COLUMN IF EXISTS type;
		ALTER TABLE ai_tools DROP COLUMN IF EXISTS category;
	`); err != nil {
		return err
	}

	return nil
}
