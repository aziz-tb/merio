// Package models defines the authenticated-user type for the auth layer.
package models

// User represents an authenticated user.
type User struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email,omitempty"`
}
