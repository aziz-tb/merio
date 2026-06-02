package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_3 restructures the AI operations audit landscape (Batch 9):
//
//   - Creates ai_operations, the unified per-logical-AI-operation outcome
//     audit, and folds tag_classifications into it.
//   - Slims conversation_tags: drops the surrogate id (natural key
//     (conversation_id, tag_id) becomes the PK), renames the
//     classification_id provenance FK to ai_operation_id (now pointing at
//     ai_operations), and drops the unread per-row timestamps.
//   - Drops the always-NULL ai_usage.stage column and adds a
//     conversation-scoped index for cross-feature "all AI calls on
//     conversation X" lookups.
//   - Drops tag_classifications.
//
// Dev one-shot: no dual-write, no backfill. Idempotent so reruns are safe.
// Note: agent_executions.updated_at is intentionally NOT dropped — it is
// still SELECTed and JSON-exposed in agent execution API responses, so the
// Batch 9 grep-before-drop rule keeps it.
func V2_8_3(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("v2_8_3: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		-- 1. ai_operations table + indexes.
		CREATE TABLE IF NOT EXISTS ai_operations (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			operation_type TEXT NOT NULL,
			conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE,
			triggered_by TEXT NOT NULL,
			triggered_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
			trace_id TEXT NOT NULL,
			status TEXT NOT NULL,
			error_message TEXT,
			payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			CONSTRAINT check_ai_operations_triggered_by CHECK (triggered_by IN ('auto', 'manual')),
			CONSTRAINT check_ai_operations_status CHECK (status IN ('running', 'success', 'failed', 'skipped'))
		);
		CREATE INDEX IF NOT EXISTS index_ai_operations_on_conversation
			ON ai_operations(conversation_id, created_at DESC) WHERE conversation_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_ai_operations_on_type_created
			ON ai_operations(operation_type, created_at DESC);
		CREATE INDEX IF NOT EXISTS index_ai_operations_on_trace
			ON ai_operations(trace_id);

		-- 2. conversation_tags: provenance FK rename classification_id -> ai_operation_id.
		ALTER TABLE conversation_tags
			ADD COLUMN IF NOT EXISTS ai_operation_id BIGINT NULL;
		ALTER TABLE conversation_tags
			DROP CONSTRAINT IF EXISTS fk_conversation_tags_classification_id;
		ALTER TABLE conversation_tags
			DROP COLUMN IF EXISTS classification_id;
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'fk_conversation_tags_ai_operation_id'
			) THEN
				ALTER TABLE conversation_tags
					ADD CONSTRAINT fk_conversation_tags_ai_operation_id
					FOREIGN KEY (ai_operation_id) REFERENCES ai_operations(id) ON DELETE SET NULL;
			END IF;
		END $$;

		-- 3. conversation_tags: drop surrogate id, promote natural key to PK.
		-- Guarded on id existing so the whole swap only runs once.
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'conversation_tags' AND column_name = 'id'
			) THEN
				ALTER TABLE conversation_tags DROP CONSTRAINT IF EXISTS conversation_tags_pkey;
				ALTER TABLE conversation_tags DROP COLUMN id;
				DROP INDEX IF EXISTS index_conversation_tags_on_conversation_id_and_tag_id;
				ALTER TABLE conversation_tags ADD PRIMARY KEY (conversation_id, tag_id);
			END IF;
		END $$;

		-- 4. conversation_tags: drop unread per-row timestamps.
		ALTER TABLE conversation_tags
			DROP COLUMN IF EXISTS created_at,
			DROP COLUMN IF EXISTS updated_at;

		-- 5. ai_usage: drop always-NULL stage, add conversation index.
		ALTER TABLE ai_usage DROP COLUMN IF EXISTS stage;
		CREATE INDEX IF NOT EXISTS index_ai_usage_on_conversation_id_created_at
			ON ai_usage (conversation_id, created_at DESC) WHERE conversation_id IS NOT NULL;

		-- 6. Drop the folded-in table.
		DROP TABLE IF EXISTS tag_classifications CASCADE;
	`); err != nil {
		return fmt.Errorf("v2_8_3: ddl: %w", err)
	}

	return tx.Commit()
}
