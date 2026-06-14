package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_6_0 prepares ai_agents for the composer layer:
//   - drops the dead per-agent confidence_threshold and autonomy columns
//     (stored-but-unread; the new design gates on neither — auto-send and
//     approval are Composer config, not per-agent knobs). DROP COLUMN also
//     removes the dependent constraint_ai_agents_on_threshold CHECK.
//   - widens agent_executions.source to allow "subagent" so a sub-agent run
//     invoked by a composer persists alongside the composer run.
//
// The composer is a platform component, not an ai_agents row, so no role
// column is added. Forward-only and idempotent (IF [NOT] EXISTS) so re-running
// is safe.
func V0_6_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE ai_agents
			DROP COLUMN IF EXISTS confidence_threshold,
			DROP COLUMN IF EXISTS autonomy;

		ALTER TABLE agent_executions
			DROP CONSTRAINT IF EXISTS constraint_agent_executions_on_source;
		ALTER TABLE agent_executions
			ADD CONSTRAINT constraint_agent_executions_on_source
			CHECK (source IN ('composer', 'pipeline', 'subagent'));
	`)
	return err
}
