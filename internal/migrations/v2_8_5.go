package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_5 dedupes legacy pending applied_slas per conversation and adds a partial
// unique index so only one pending SLA can exist per conversation. Ported from the
// SLA portion of upstream's v2.3.0 migration (PR #333); locale changes intentionally omitted.
func V2_8_5(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE indexname = 'index_applied_slas_unique_pending_per_conv'
				  AND tablename = 'applied_slas'
			) THEN
				WITH ranked AS (
					SELECT id, row_number() OVER (PARTITION BY conversation_id ORDER BY created_at DESC, id DESC) AS rn
					FROM applied_slas
					WHERE status = 'pending'
				)
				DELETE FROM applied_slas WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

				CREATE UNIQUE INDEX index_applied_slas_unique_pending_per_conv
				ON applied_slas(conversation_id) WHERE status = 'pending';
			END IF;
		END $$;
	`); err != nil {
		return fmt.Errorf("v2_8_5: dedupe pending applied_slas + unique index: %w", err)
	}
	return nil
}
