package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_24 makes inbound email Message-ID deduplication race-safe.
func V2_8_24(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	stmts := []struct {
		name string
		sql  string
	}{
		{
			name: "report duplicate source ids",
			sql: `
				DO $$
				DECLARE
					dup_count INT;
				BEGIN
					SELECT COUNT(*) INTO dup_count FROM (
						SELECT source_id
						FROM conversation_messages
						WHERE source_id IS NOT NULL
						GROUP BY source_id
						HAVING COUNT(*) > 1
					) dupes;
					IF dup_count > 0 THEN
						RAISE NOTICE 'Found % duplicate source_id values that will be cleaned up before unique index creation', dup_count;
					END IF;
				END $$;
			`,
		},
		{
			name: "delete duplicate messages in kept conversations",
			sql: `
				WITH ranked AS (
					SELECT
						id,
						conversation_id,
						source_id,
						ROW_NUMBER() OVER (PARTITION BY source_id ORDER BY created_at ASC, id ASC) AS rn
					FROM conversation_messages
					WHERE source_id IS NOT NULL
				),
				to_delete AS (
					SELECT r.id
					FROM ranked r
					JOIN ranked keep ON keep.source_id = r.source_id AND keep.rn = 1
					WHERE r.rn > 1
					  AND r.conversation_id = keep.conversation_id
				)
				DELETE FROM conversation_messages
				WHERE id IN (SELECT id FROM to_delete);
			`,
		},
		{
			name: "delete duplicate conversations",
			sql: `
				WITH ranked AS (
					SELECT
						id,
						conversation_id,
						source_id,
						ROW_NUMBER() OVER (PARTITION BY source_id ORDER BY created_at ASC, id ASC) AS rn
					FROM conversation_messages
					WHERE source_id IS NOT NULL
				),
				to_delete AS (
					SELECT DISTINCT r.conversation_id
					FROM ranked r
					JOIN ranked keep ON keep.source_id = r.source_id AND keep.rn = 1
					WHERE r.rn > 1
					  AND r.conversation_id <> keep.conversation_id
				)
				DELETE FROM conversations
				WHERE id IN (SELECT conversation_id FROM to_delete);
			`,
		},
		{
			name: "replace non-unique source id index",
			sql:  `DROP INDEX IF EXISTS index_conversation_messages_on_source_id;`,
		},
		{
			name: "create unique source id index",
			sql: `
				CREATE UNIQUE INDEX IF NOT EXISTS index_unique_conversation_messages_on_source_id
					ON conversation_messages(source_id)
					WHERE source_id IS NOT NULL;
			`,
		},
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt.sql); err != nil {
			return fmt.Errorf("v2_8_24: %s: %w", stmt.name, err)
		}
	}
	return nil
}
