package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_11 grants the customer-identification agent permissions to the
// seeded Agent role (design Section 14: "all agents get customers:assign
// and customers:search"). v2_8_10 already granted all three to Admin;
// this finishes the default bindings for the Agent role. customers:manage
// stays Admin-only (it gates admin config).
//
// Idempotent via the same NOT (... = ANY(permissions)) pattern v2_7_4 /
// v2_8_10 use, applied per permission. Forward-only. Rollback (manual):
//
//	UPDATE roles SET permissions = ARRAY(
//	    SELECT UNNEST(permissions)
//	    EXCEPT SELECT unnest(ARRAY['customers:assign','customers:search'])
//	) WHERE name = 'Agent';
func V2_8_11(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, perm := range []string{"customers:assign", "customers:search"} {
		if _, err := db.Exec(`
			UPDATE roles
			SET    permissions = permissions || ARRAY[$1]::TEXT[]
			WHERE  name = 'Agent'
			  AND  NOT ($1 = ANY(permissions));
		`, perm); err != nil {
			return err
		}
	}
	return nil
}
