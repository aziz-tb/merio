// Package models defines the role type and the built-in role names.
package models

import (
	"time"

	"github.com/lib/pq"
)

// Built-in role names.
const (
	RoleAdmin = "Admin"
	RoleAgent = "Agent"
)

// DefaultRoles lists the built-in roles; the manager refuses to delete them.
var DefaultRoles = []string{
	RoleAdmin,
	RoleAgent,
}

// Role is a named set of permissions assignable to users; Permissions holds authz
// permission keys.
type Role struct {
	ID          int            `db:"id" json:"id"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`
	Name        string         `db:"name" json:"name"`
	Description string         `db:"description" json:"description"`
	Permissions pq.StringArray `db:"permissions" json:"permissions"`
}
