// Package models defines the activity-log entry type and its activity-type constants.
package models

import (
	"time"
)

// Activity types recorded in ActivityLog.ActivityType.
const (
	AgentLogin                  = "agent_login"
	AgentLogout                 = "agent_logout"
	AgentAway                   = "agent_away"
	AgentAwayReassigned         = "agent_away_reassigned"
	AgentOnline                 = "agent_online"
	AgentPasswordSet            = "agent_password_set"
	AgentRolePermissionsChanged = "agent_role_permissions_changed"
)

// ActivityLog is a single audit-log entry for an agent action.
type ActivityLog struct {
	ID                  int64     `db:"id" json:"id"`
	CreatedAt           time.Time `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time `db:"updated_at" json:"updated_at"`
	ActivityType        string    `db:"activity_type" json:"activity_type"`
	ActivityDescription string    `db:"activity_description" json:"activity_description"`
	ActorID             int       `db:"actor_id" json:"actor_id"`
	TargetModelType     string    `db:"target_model_type" json:"target_model_type"`
	TargetModelID       int       `db:"target_model_id" json:"target_model_id"`
	IP                  string    `db:"ip" json:"ip"`

	// Total is the list-query row count, for pagination (not a stored column).
	Total int `db:"total" json:"-"`
}
