// Package models contains the data models for the function package.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Domain values for functions.domain. The set is enforced by the
// check_functions_domain CHECK constraint; keep this list in sync with it.
const (
	DomainAITool                 = "ai_tool"
	DomainCustomerIdentification = "customer_identification"
)

// Function is the DB-wire representation of a functions row — one named,
// sandbox-runnable code blob. The struct mirrors the functions table
// verbatim. Code/Language/Packages/TimeoutMS feed the F6 exec runner;
// Parameters is the {id, key, value} array used by preview/test runs.
type Function struct {
	ID              int64           `db:"id" json:"id"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	Name            string          `db:"name" json:"name"`
	Domain          string          `db:"domain" json:"domain"`
	Code            string          `db:"code" json:"code"`
	Language        string          `db:"language" json:"language"`
	Packages        string          `db:"packages" json:"packages"`
	TimeoutMS       int             `db:"timeout_ms" json:"timeout_ms"`
	Parameters      json.RawMessage `db:"parameters" json:"parameters"`
	UpdatedByUserID null.Int        `db:"updated_by_user_id" json:"updated_by_user_id"`
}
