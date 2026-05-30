// Package models contains the data models for the secret package.
package models

import (
	"time"

	"github.com/volatiletech/null/v9"
)

// Secret is the metadata-only API/DB representation of a platform secret.
// encrypted_value and nonce are intentionally omitted from this public shape;
// plaintext secret values are never serialized.
type Secret struct {
	ID              int64     `db:"id" json:"-"`
	Name            string    `db:"name" json:"name"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
	LastAccessedAt  null.Time `db:"last_accessed_at" json:"last_accessed_at"`
	CreatedByUserID null.Int  `db:"created_by_user_id" json:"created_by_user_id"`
}
