package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_2 augments tags for AI classification and records tag classification
// audit rows.
func V2_8_2(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("v2_8_2: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		ALTER TABLE tags
			ADD COLUMN IF NOT EXISTS parent_id INT NULL REFERENCES tags(id) ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'skill',
			ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT 'grey',
			ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public',
			ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS detection JSONB NOT NULL DEFAULT '{"mode":"manual_only"}'::jsonb,
			ADD COLUMN IF NOT EXISTS sla_policy_id INT NULL REFERENCES sla_policies(id) ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS priority_id INT NULL REFERENCES conversation_priorities(id) ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS updated_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_tags_on_type') THEN
				ALTER TABLE tags ADD CONSTRAINT constraint_tags_on_type CHECK (type IN ('skill', 'topic', 'contact', 'other'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_tags_on_visibility') THEN
				ALTER TABLE tags ADD CONSTRAINT constraint_tags_on_visibility CHECK (visibility IN ('public', 'private', 'disabled'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_tags_on_color') THEN
				ALTER TABLE tags ADD CONSTRAINT constraint_tags_on_color CHECK (color IN ('grey', 'green', 'red', 'blue', 'yellow', 'purple', 'orange', 'teal'));
			END IF;
		END $$;

		CREATE INDEX IF NOT EXISTS index_tags_on_parent_id ON tags(parent_id) WHERE parent_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS index_tags_on_visibility ON tags(visibility);
		CREATE INDEX IF NOT EXISTS index_tags_on_detection_mode ON tags((detection->>'mode'));
		CREATE INDEX IF NOT EXISTS index_tags_on_updated_by ON tags(updated_by) WHERE updated_by IS NOT NULL;

		ALTER TABLE conversation_tags
			ADD COLUMN IF NOT EXISTS applied_by TEXT NOT NULL DEFAULT 'manual',
			ADD COLUMN IF NOT EXISTS classification_id BIGINT NULL;

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_conversation_tags_on_applied_by') THEN
				ALTER TABLE conversation_tags ADD CONSTRAINT constraint_conversation_tags_on_applied_by CHECK (applied_by IN ('manual', 'ai', 'rule'));
			END IF;
		END $$;

		CREATE INDEX IF NOT EXISTS index_conversation_tags_on_applied_by ON conversation_tags(applied_by);

		CREATE TABLE IF NOT EXISTS tag_classifications (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			triggered_by TEXT NOT NULL,
			triggered_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
			llm_reasoning TEXT,
			llm_scores JSONB,
			llm_model TEXT,
			llm_input_tokens INT,
			llm_output_tokens INT,
			llm_latency_ms INT,
			candidate_tag_ids INT[] NOT NULL DEFAULT '{}',
			applied_tag_ids INT[] NOT NULL DEFAULT '{}',
			removed_tag_ids INT[] NOT NULL DEFAULT '{}',
			status TEXT NOT NULL,
			error_message TEXT,
			CONSTRAINT constraint_tag_classifications_on_triggered_by CHECK (triggered_by IN ('auto', 'manual')),
			CONSTRAINT constraint_tag_classifications_on_status CHECK (status IN ('success', 'failed', 'skipped'))
		);
		CREATE INDEX IF NOT EXISTS index_tag_classifications_on_conv ON tag_classifications(conversation_id);
		CREATE INDEX IF NOT EXISTS index_tag_classifications_on_created_at ON tag_classifications(created_at DESC);

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_conversation_tags_classification_id') THEN
				ALTER TABLE conversation_tags
					ADD CONSTRAINT fk_conversation_tags_classification_id
					FOREIGN KEY (classification_id) REFERENCES tag_classifications(id) ON DELETE SET NULL;
			END IF;
		END $$;

		UPDATE roles
		SET permissions = permissions || ARRAY['tags:view_private']::TEXT[]
		WHERE name = 'Admin'
		  AND NOT ('tags:view_private' = ANY(permissions));
	`); err != nil {
		return fmt.Errorf("v2_8_2: ddl: %w", err)
	}

	return tx.Commit()
}
