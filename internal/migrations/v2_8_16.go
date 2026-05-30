package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_16 disables empty customer-identification function seeds. v2_8_14
// originally created them enabled, but an enabled empty function fails with
// ErrFunctionCodeEmpty on first runtime use. Operators enable each function
// after adding code in the admin UI.
func V2_8_16(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		UPDATE functions
		SET    enabled = FALSE
		WHERE  domain = 'customer_identification'
		  AND  code = ''
		  AND  enabled = TRUE;
	`)
	return err
}
