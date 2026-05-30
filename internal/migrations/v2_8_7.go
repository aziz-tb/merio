package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_7 drops ai_tools.min_legitimation (design Section 10.1). Tool-level
// legitimation gating is removed in favour of agent-level gating: if the
// agent is allowed to run, its tools run; the agent's own
// min_legitimation (added in v2_7_5) is the single gate. The associated
// CHECK constraint is dropped first; Postgres would cascade it on the
// column drop anyway, but the explicit DROP keeps the intent obvious.
//
// Idempotent via DROP CONSTRAINT / COLUMN IF EXISTS. Forward-only.
// Rollback (manual):
//
//	ALTER TABLE ai_tools ADD COLUMN IF NOT EXISTS min_legitimation INT NOT NULL DEFAULT 0;
//	ALTER TABLE ai_tools ADD CONSTRAINT constraint_ai_tools_on_legit CHECK (min_legitimation >= 0);
func V2_8_7(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		ALTER TABLE ai_tools DROP CONSTRAINT IF EXISTS constraint_ai_tools_on_legit;
		ALTER TABLE ai_tools DROP COLUMN IF EXISTS min_legitimation;
	`); err != nil {
		return err
	}
	return nil
}
