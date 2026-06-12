// Package models defines the conversation status type and its built-in defaults.
package models

import "time"

// DefaultStatuses are the status names seeded on setup; the manager refuses to
// rename or delete them.
var DefaultStatuses = []string{
	"Open",
	"Snoozed",
	"Resolved",
	"Closed",
}

// Status categories. A Status.Category must be one of these, and the category also
// selects the default color and icon.
const (
	CategoryOpen     = "open"
	CategoryWaiting  = "waiting"
	CategoryResolved = "resolved"
)

// Default color and icon per category, used when a status sets neither.
const (
	DefaultColorOpen     = "blue"
	DefaultColorWaiting  = "yellow"
	DefaultColorResolved = "green"
	DefaultIconOpen      = "CircleDot"
	DefaultIconWaiting   = "Moon"
	DefaultIconResolved  = "CircleCheck"
)

// ValidCategories is the allowlist for Status.Category.
var ValidCategories = []string{
	CategoryOpen,
	CategoryWaiting,
	CategoryResolved,
}

// Status is a configurable conversation status. Category buckets it; Color and Icon
// fall back to per-category defaults when unset.
type Status struct {
	ID        int       `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	Name      string    `db:"name" json:"name"`
	Category  string    `db:"category" json:"category"`
	Color     string    `db:"color" json:"color"`
	Icon      string    `db:"icon" json:"icon"`
}
