package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_0 updates agent_executions for the Batch 4 native-thinking +
// produce_draft contract:
//
//   - format_version marks legacy rows as "1" and new rows as "2"
//   - draft becomes nullable so runs that never call produce_draft can
//     persist a loud failure with draft=NULL
//
// Idempotency notes:
//   - format_version is backfilled only where NULL, so rerunning the
//     migration does not rewrite new rows that already carry "2"
//   - dropping draft's NOT NULL / DEFAULT is metadata-only and safe to
//     repeat
func V2_8_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		ALTER TABLE agent_executions
			ADD COLUMN IF NOT EXISTS format_version TEXT
	`); err != nil {
		return fmt.Errorf("v2_8_0: add agent_executions.format_version: %w", err)
	}

	if _, err := db.Exec(`
		UPDATE agent_executions
		SET    format_version = '1'
		WHERE  format_version IS NULL
	`); err != nil {
		return fmt.Errorf("v2_8_0: backfill agent_executions.format_version: %w", err)
	}

	if _, err := db.Exec(`
		ALTER TABLE agent_executions
			ALTER COLUMN format_version SET DEFAULT '2',
			ALTER COLUMN format_version SET NOT NULL,
			ALTER COLUMN draft DROP NOT NULL,
			ALTER COLUMN draft DROP DEFAULT
	`); err != nil {
		return fmt.Errorf("v2_8_0: alter agent_executions columns: %w", err)
	}

	return nil
}
