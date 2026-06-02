package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_26 removes the deprecated context links feature. The table-backed
// feature has been superseded by the platform extensibility plan.
func V2_8_26(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		DROP TABLE IF EXISTS context_links CASCADE;

		UPDATE roles
		SET permissions = array_remove(permissions, 'context_links:manage')
		WHERE 'context_links:manage' = ANY(permissions);
	`)
	return err
}
