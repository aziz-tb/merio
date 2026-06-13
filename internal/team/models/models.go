// Package models defines the team type and its compact projections.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/volatiletech/null/v9"
)

// Team is an agent team with its assignment strategy, business hours, SLA policy,
// and auto-assignment cap.
type Team struct {
	ID                           int         `db:"id" json:"id"`
	CreatedAt                    time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt                    time.Time   `db:"updated_at" json:"updated_at"`
	Emoji                        null.String `db:"emoji" json:"emoji"`
	Name                         string      `db:"name" json:"name"`
	ConversationAssignmentType   string      `db:"conversation_assignment_type" json:"conversation_assignment_type"`
	Timezone                     string      `db:"timezone" json:"timezone"`
	BusinessHoursID              null.Int    `db:"business_hours_id" json:"business_hours_id"`
	SLAPolicyID                  null.Int    `db:"sla_policy_id" json:"sla_policy_id"`
	MaxAutoAssignedConversations int         `db:"max_auto_assigned_conversations" json:"max_auto_assigned_conversations"`
}

// TeamCompact is a minimal team reference (id, name, emoji).
type TeamCompact struct {
	ID    int         `db:"id" json:"id"`
	Name  string      `db:"name" json:"name"`
	Emoji null.String `db:"emoji" json:"emoji"`
}

// TeamMember is a team membership: a user, their team, and their availability.
type TeamMember struct {
	ID                 int    `db:"id" json:"id"`
	AvailabilityStatus string `db:"availability_status" json:"availability_status"`
	TeamID             int    `db:"team_id" json:"team_id"`
}

// TeamsCompact is a list of teams stored as JSON in a single column (see Scan/Value).
type TeamsCompact []TeamCompact

// IDs returns the team IDs.
func (t TeamsCompact) IDs() []int {
	ids := make([]int, len(t))
	for i, team := range t {
		ids[i] = team.ID
	}
	return ids
}

// Scan implements the sql.Scanner interface for Teams
func (t *TeamsCompact) Scan(src interface{}) error {
	if src == nil {
		*t = nil
		return nil
	}

	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, t)
	default:
		return fmt.Errorf("unsupported type for Teams: %T", src)
	}
}

// Value implements the driver.Valuer interface for Teams
func (t TeamsCompact) Value() (driver.Value, error) {
	return json.Marshal(t)
}
