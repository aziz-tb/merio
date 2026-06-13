// Package models defines the custom-attribute definition type.
package models

import (
	"time"

	"github.com/lib/pq"
)

// CustomAttribute is an admin-defined custom field; AppliesTo selects the entity it attaches
// to, and DataType/Regex constrain its values.
type CustomAttribute struct {
	ID          int            `db:"id" json:"id"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`
	Name        string         `db:"name" json:"name"`
	Description string         `db:"description" json:"description"`
	AppliesTo   string         `db:"applies_to" json:"applies_to"`
	Key         string         `db:"key" json:"key"`
	Values      pq.StringArray `db:"values" json:"values"`
	DataType    string         `db:"data_type" json:"data_type"`
	Regex       string         `db:"regex" json:"regex"`
	RegexHint   string         `db:"regex_hint" json:"regex_hint"`
}
