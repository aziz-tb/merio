package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_10 grants the three customer-identification permissions to the
// seeded Admin role (design Section 14):
//
//	customers:assign  -- manually assign / switch a conversation's contract
//	customers:search  -- use the free-text customer search box
//	customers:manage  -- admin config (executors, rules, search params)
//
// Idempotent via the same NOT (... = ANY(permissions)) pattern v2_7_4
// uses, applied per permission. Forward-only. Rollback (manual):
//
//	UPDATE roles SET permissions = ARRAY(
//	    SELECT UNNEST(permissions)
//	    EXCEPT SELECT unnest(ARRAY['customers:assign','customers:search','customers:manage'])
//	) WHERE name = 'Admin';
func V2_8_10(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, perm := range []string{"customers:assign", "customers:search", "customers:manage"} {
		if _, err := db.Exec(`
			UPDATE roles
			SET    permissions = permissions || ARRAY[$1]::TEXT[]
			WHERE  name = 'Admin'
			  AND  NOT ($1 = ANY(permissions));
		`, perm); err != nil {
			return err
		}
	}
	return nil
}
