package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_8 renames tool_executions → function_executions (design Section
// 3.6). Mechanical rename, no column changes — the table is becoming the
// audit surface for both AI tool calls and the customer-identification
// code executors, where "function" reads better than "tool". RENAME TO
// preserves all rows and indexes.
//
// Postgres does NOT auto-rename indexes on a table rename, so — as the
// earlier script_executions → tool_executions rename (v2_7_2) did — we
// rename the four indexes explicitly so `\d function_executions` doesn't
// surface stale idx_tool_executions_* names.
//
// The IF EXISTS / DO guards make the migration safe on a fresh schema
// (where tool_executions never existed, function_executions is already
// present) and on a re-run (rename is a no-op).
//
// Idempotent. Forward-only. Rollback: rename in the opposite direction.
func V2_8_8(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. tool_executions → function_executions.
	if _, err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables
			           WHERE table_schema = current_schema() AND table_name = 'tool_executions') THEN
				ALTER TABLE tool_executions RENAME TO function_executions;
			END IF;
		END $$;
	`); err != nil {
		return err
	}

	// 2. Rename the indexes to match the table.
	for _, idx := range []struct {
		oldName string
		newName string
	}{
		{"idx_tool_executions_tool_name", "idx_function_executions_tool_name"},
		{"idx_tool_executions_conversation_id", "idx_function_executions_conversation_id"},
		{"idx_tool_executions_started_at", "idx_function_executions_started_at"},
		{"idx_tool_executions_invoker_user_id", "idx_function_executions_invoker_user_id"},
	} {
		if _, err := db.Exec(`ALTER INDEX IF EXISTS ` + idx.oldName + ` RENAME TO ` + idx.newName + `;`); err != nil {
			return err
		}
	}

	return nil
}
