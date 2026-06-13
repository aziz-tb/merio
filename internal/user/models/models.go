// Package models defines the user type (agents, contacts, visitors) and related constants.
package models

import (
	"encoding/json"
	"slices"
	"time"

	rmodels "github.com/aziz-tb/merio/internal/role/models"
	tmodels "github.com/aziz-tb/merio/internal/team/models"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
)

// User-related constants: the user-model key, the built-in system account, user types,
// and availability statuses (Away is automatic/inactivity; AwayManual is set from the sidebar).
const (
	UserModel = "user"

	SystemUserEmail = "system@merio.local"

	UserTypeAgent   = "agent"
	UserTypeContact = "contact"
	UserTypeVisitor = "visitor"

	Online             = "online"
	Offline            = "offline"
	Away               = "away"
	AwayManual         = "away_manual"
	AwayAndReassigning = "away_and_reassigning"
)

// UserCompact is a reduced user projection for list views.
type UserCompact struct {
	ID             int         `db:"id" json:"id"`
	Type           string      `db:"type" json:"type"`
	FirstName      string      `db:"first_name" json:"first_name"`
	LastName       string      `db:"last_name" json:"last_name"`
	Email          null.String `db:"email" json:"email"`
	Enabled        bool        `db:"enabled" json:"enabled"`
	IsSystem       bool        `db:"is_system" json:"is_system"`
	AvatarURL      null.String `db:"avatar_url" json:"avatar_url"`
	ExternalUserID null.String `db:"external_user_id" json:"external_user_id"`
	CreatedAt      time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at" json:"updated_at"`

	Total int `db:"total" json:"-"`
}

// LinkedContact is a minimal contact reference (id, name, email).
type LinkedContact struct {
	ID        int         `db:"id" json:"id"`
	Email     null.String `db:"email" json:"email"`
	FirstName string      `db:"first_name" json:"first_name"`
	LastName  string      `db:"last_name" json:"last_name"`
}

// User is an agent, contact, or visitor account — with roles and permissions, customer-identity
// link state, and optional API-key credentials.
type User struct {
	ID                     int                  `db:"id" json:"id"`
	CreatedAt              time.Time            `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time            `db:"updated_at" json:"updated_at"`
	FirstName              string               `db:"first_name" json:"first_name"`
	LastName               string               `db:"last_name" json:"last_name"`
	Email                  null.String          `db:"email" json:"email"`
	Type                   string               `db:"type" json:"type"`
	AvailabilityStatus     string               `db:"availability_status" json:"availability_status"`
	PhoneNumberCountryCode null.String          `db:"phone_number_country_code" json:"phone_number_country_code"`
	PhoneNumber            null.String          `db:"phone_number" json:"phone_number"`
	AvatarURL              null.String          `db:"avatar_url" json:"avatar_url"`
	Enabled                bool                 `db:"enabled" json:"enabled"`
	IsSystem               bool                 `db:"is_system" json:"is_system"`
	Password               null.String          `db:"password" json:"-"`
	LastActiveAt           null.Time            `db:"last_active_at" json:"last_active_at"`
	LastLoginAt            null.Time            `db:"last_login_at" json:"last_login_at"`
	Roles                  pq.StringArray       `db:"roles" json:"roles"`
	Permissions            pq.StringArray       `db:"permissions" json:"permissions"`
	Country                null.String          `db:"country" json:"country"`
	Meta                   json.RawMessage      `db:"meta" json:"meta"`
	CustomAttributes       json.RawMessage      `db:"custom_attributes" json:"custom_attributes"`
	ExternalUserID         null.String          `db:"external_user_id" json:"external_user_id"`
	LinkedCustomerID       null.String          `db:"linked_customer_id" json:"linked_customer_id"`
	LinkedCustomerIDAt     null.Time            `db:"linked_customer_id_at" json:"linked_customer_id_at"`
	Teams                  tmodels.TeamsCompact `db:"teams" json:"teams"`
	ContactChannelID       int                  `db:"contact_channel_id" json:"contact_channel_id,omitempty"`
	NewPassword            string               `db:"-" json:"new_password,omitempty"`
	SendWelcomeEmail       bool                 `db:"-" json:"send_welcome_email,omitempty"`
	InboxID                int                  `json:"-"`
	SourceChannel          null.String          `json:"-"`
	SourceChannelID        null.String          `json:"-"`

	// API Key fields
	APIKey           null.String `db:"api_key" json:"api_key"`
	APIKeyLastUsedAt null.Time   `db:"api_key_last_used_at" json:"api_key_last_used_at"`
	APISecret        null.String `db:"api_secret" json:"-"`
}

// ChatUser is a user with limited fields for live chat.
type ChatUser struct {
	ID                 int         `db:"id" json:"id"`
	FirstName          string      `db:"first_name" json:"first_name"`
	LastName           string      `db:"last_name" json:"last_name"`
	AvatarURL          null.String `db:"avatar_url" json:"avatar_url"`
	AvailabilityStatus string      `db:"availability_status" json:"availability_status"`
	Type               string      `db:"type" json:"type"`
	ActiveAt           null.Time   `db:"active_at" json:"active_at"`
}

// Note is an internal note attached to a contact.
type Note struct {
	ID        int         `db:"id" json:"id"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt time.Time   `db:"updated_at" json:"updated_at"`
	ContactID int         `db:"contact_id" json:"contact_id"`
	Note      string      `db:"note" json:"note"`
	UserID    int         `db:"user_id" json:"user_id"`
	FirstName string      `db:"first_name" json:"first_name"`
	LastName  string      `db:"last_name" json:"last_name"`
	AvatarURL null.String `db:"avatar_url" json:"avatar_url"`
}

// OfflineUser is a minimal reference (id, type) to a user that is offline.
type OfflineUser struct {
	ID   int    `db:"id"`
	Type string `db:"type"`
}

// FullName returns the user's first and last name joined.
func (u *User) FullName() string {
	return u.FirstName + " " + u.LastName
}

// HasAdminRole reports whether the user holds the Admin role.
func (u *User) HasAdminRole() bool {
	return slices.Contains(u.Roles, rmodels.RoleAdmin)
}

// IsSystemUser reports whether the user is the built-in system account, by flag or system email.
func (u *User) IsSystemUser() bool {
	return u.IsSystem || u.Email.String == SystemUserEmail
}
