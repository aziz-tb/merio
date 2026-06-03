package models

import "time"

var DefaultStatuses = []string{
	"Open",
	"Snoozed",
	"Resolved",
	"Closed",
}

const (
	CategoryOpen     = "open"
	CategoryWaiting  = "waiting"
	CategoryResolved = "resolved"
)

const (
	DefaultColorOpen     = "blue"
	DefaultColorWaiting  = "yellow"
	DefaultColorResolved = "green"
	DefaultIconOpen      = "CircleDot"
	DefaultIconWaiting   = "Moon"
	DefaultIconResolved  = "CircleCheck"
)

var ValidCategories = []string{
	CategoryOpen,
	CategoryWaiting,
	CategoryResolved,
}

type Status struct {
	ID        int       `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	Name      string    `db:"name" json:"name"`
	Category  string    `db:"category" json:"category"`
	Color     string    `db:"color" json:"color"`
	Icon      string    `db:"icon" json:"icon"`
}
