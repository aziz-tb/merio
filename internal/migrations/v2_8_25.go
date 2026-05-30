package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_25 adds the contact-primary linked customer memory. Conversations can
// now inherit a contact's confirmed customer as a soft prior.
func V2_8_25(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE users
			ADD COLUMN IF NOT EXISTS linked_customer_id TEXT
				REFERENCES customers(customer_id)
				ON UPDATE CASCADE
				ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS linked_customer_id_at TIMESTAMPTZ;

		CREATE INDEX IF NOT EXISTS index_users_on_linked_customer_id
			ON users(linked_customer_id)
			WHERE linked_customer_id IS NOT NULL;
	`)
	return err
}
