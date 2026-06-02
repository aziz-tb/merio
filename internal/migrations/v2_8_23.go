package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_23 adds encrypted operator-managed platform secrets and grants the
// seeded Admin role permission to manage them.
func V2_8_23(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS secrets (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			encrypted_value BYTEA NOT NULL,
			nonce BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_accessed_at TIMESTAMPTZ,
			created_by_user_id INT REFERENCES users(id) ON DELETE SET NULL,
			CONSTRAINT secrets_name_format CHECK (name ~ '^[a-z][a-z0-9_]{0,63}$')
		);

		CREATE INDEX IF NOT EXISTS index_secrets_on_name ON secrets(name);

		UPDATE roles
		SET    permissions = permissions || ARRAY['secrets:manage']::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('secrets:manage' = ANY(permissions));
	`)
	return err
}
