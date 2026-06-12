// Package models defines the message template type.
package models

import (
	"time"

	"github.com/volatiletech/null/v9"
)

// Template is a stored message template. Type identifies its kind; IsDefault marks the
// default for that type (one per type), and IsBuiltIn marks a shipped template that
// cannot be deleted.
type Template struct {
	ID        int         `db:"id" json:"id"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt time.Time   `db:"updated_at" json:"updated_at"`
	Type      string      `db:"type" json:"type"`
	Name      string      `db:"name" json:"name"`
	Subject   null.String `db:"subject" json:"subject"`
	Body      string      `db:"body" json:"body"`
	IsDefault bool        `db:"is_default" json:"is_default"`
	IsBuiltIn bool        `db:"is_builtin" json:"is_builtin"`
}
