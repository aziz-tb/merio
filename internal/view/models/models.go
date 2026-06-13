// Package models defines the saved-view type and its visibility constants.
package models

import (
	"encoding/json"
	"time"
)

// Visibility constants for views
const (
	VisibilityAll  = "all"
	VisibilityTeam = "team"
	VisibilityUser = "user"
)

// View is a saved, filtered conversation view, scoped by Visibility to all, a team, or a user.
type View struct {
	ID         int             `db:"id" json:"id"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
	Name       string          `db:"name" json:"name"`
	Filters    json.RawMessage `db:"filters" json:"filters"`
	Visibility string          `db:"visibility" json:"visibility"`
	UserID     *int            `db:"user_id" json:"user_id,omitempty"`
	TeamID     *int            `db:"team_id" json:"team_id,omitempty"`
}
