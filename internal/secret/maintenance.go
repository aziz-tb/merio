package secret

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type maintenanceRow struct {
	ID             int64  `db:"id"`
	Name           string `db:"name"`
	EncryptedValue []byte `db:"encrypted_value"`
	Nonce          []byte `db:"nonce"`
}

// RotateKEK re-encrypts every secret with newKEK inside a single transaction.
func RotateKEK(ctx context.Context, db *sqlx.DB, oldKEK, newKEK []byte) (int, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var rows []maintenanceRow
	if err := tx.SelectContext(ctx, &rows, `
		SELECT id, name, encrypted_value, nonce
		FROM secrets
		ORDER BY id ASC
		FOR UPDATE
	`); err != nil {
		return 0, err
	}

	for _, row := range rows {
		plain, err := Decrypt(oldKEK, row.EncryptedValue, row.Nonce)
		if err != nil {
			return 0, fmt.Errorf("decrypt secret %q: %w", row.Name, err)
		}
		ciphertext, nonce, err := Encrypt(newKEK, plain)
		if err != nil {
			return 0, fmt.Errorf("encrypt secret %q: %w", row.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE secrets
			SET encrypted_value = $2, nonce = $3, updated_at = NOW()
			WHERE id = $1
		`, row.ID, ciphertext, nonce); err != nil {
			return 0, fmt.Errorf("update secret %q: %w", row.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// VerifyKEK attempts to decrypt all secrets with kek and returns the names of
// rows that failed to decrypt.
func VerifyKEK(ctx context.Context, db *sqlx.DB, kek []byte) ([]string, error) {
	var rows []maintenanceRow
	if err := db.SelectContext(ctx, &rows, `
		SELECT id, name, encrypted_value, nonce
		FROM secrets
		ORDER BY id ASC
	`); err != nil {
		return nil, err
	}
	failures := make([]string, 0)
	for _, row := range rows {
		if _, err := Decrypt(kek, row.EncryptedValue, row.Nonce); err != nil {
			failures = append(failures, row.Name)
		}
	}
	return failures, nil
}
