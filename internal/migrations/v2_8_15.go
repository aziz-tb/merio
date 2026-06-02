package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_15 grants the functions:manage permission to the seeded Admin role
// (Unified Executor Framework batch). A single permission gates all function
// CRUD + preview. It is NOT granted to the Agent role — agents don't manage
// code.
//
// Idempotent via the NOT (... = ANY(permissions)) pattern v2_7_4 / v2_8_10
// use. Forward-only. Rollback (manual):
//
//	UPDATE roles SET permissions = ARRAY(
//	    SELECT UNNEST(permissions) EXCEPT SELECT 'functions:manage'
//	) WHERE name = 'Admin';
func V2_8_15(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY['functions:manage']::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('functions:manage' = ANY(permissions));
	`); err != nil {
		return err
	}
	return nil
}
