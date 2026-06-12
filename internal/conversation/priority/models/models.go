// Package models defines the conversation priority type.
package models

import "time"

// Priority is a configurable conversation priority level.
type Priority struct {
	ID        int       `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	Name      string    `db:"name" json:"name"`
}
