package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_29 adds per-message send inbox overrides.
func V2_8_29(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE conversation_messages
		ADD COLUMN IF NOT EXISTS send_inbox_id INT NULL
		REFERENCES inboxes(id) ON DELETE RESTRICT ON UPDATE CASCADE;

		CREATE INDEX IF NOT EXISTS index_conversation_messages_on_send_inbox_id
		ON conversation_messages (send_inbox_id)
		WHERE send_inbox_id IS NOT NULL;
	`)
	return err
}
