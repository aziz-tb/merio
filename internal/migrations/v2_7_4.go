package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_4 grants the F7+F5 agent runner permission `ai_agents:run` to the
// seeded Admin role. Distinct from ai_agents:manage so token-spending
// authority can be granted independently in the future. Idempotent via
// the same NOT (... = ANY(permissions)) pattern v2_5_0 / v2_6_0 use.
//
// Forward-only. Rollback: `UPDATE roles SET permissions = ARRAY(SELECT
// UNNEST(permissions) EXCEPT SELECT 'ai_agents:run') WHERE name='Admin';`
func V2_7_4(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY['ai_agents:run']::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('ai_agents:run' = ANY(permissions));
	`); err != nil {
		return err
	}
	return nil
}
