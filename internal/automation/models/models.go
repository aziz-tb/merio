// Package models defines automation rule types and the action, event, and rule-type
// constants, plus the action-to-permission map.
package models

import (
	"encoding/json"
	"time"

	authzModels "github.com/aziz-tb/merio/internal/authz/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/lib/pq"
)

// Automation action types, rule trigger types, rule event names, and execution modes.
const (
	ActionAssignTeam      = "assign_team"
	ActionAssignUser      = "assign_user"
	ActionSetStatus       = "set_status"
	ActionSetPriority     = "set_priority"
	ActionSendPrivateNote = "send_private_note"
	ActionReply           = "send_reply"
	ActionSetSLA          = "set_sla"
	ActionAddTags         = "add_tags"
	ActionSetTags         = "set_tags"
	ActionRemoveTags      = "remove_tags"
	ActionSendCSAT        = "send_csat"

	RuleTypeNewConversation    = "new_conversation"
	RuleTypeConversationUpdate = "conversation_update"
	RuleTypeTimeTrigger        = "time_trigger"

	EventConversationUserAssigned    = "conversation.user.assigned"
	EventConversationTeamAssigned    = "conversation.team.assigned"
	EventConversationStatusChange    = "conversation.status.change"
	EventConversationPriorityChange  = "conversation.priority.change"
	EventConversationMessageOutgoing = "conversation.message.outgoing"
	EventConversationMessageIncoming = "conversation.message.incoming"

	ExecutionModeAll        = "all"
	ExecutionModeFirstMatch = "first_match"
)

// ActionPermissions maps each action to the permission an actor must hold to run it.
var ActionPermissions = map[string]string{
	ActionAssignTeam:      authzModels.PermConversationsUpdateTeamAssignee,
	ActionAssignUser:      authzModels.PermConversationsUpdateUserAssignee,
	ActionSetStatus:       authzModels.PermConversationsUpdateStatus,
	ActionSetPriority:     authzModels.PermConversationsUpdatePriority,
	ActionSendPrivateNote: authzModels.PermMessagesWrite,
	ActionReply:           authzModels.PermMessagesWrite,
	ActionAddTags:         authzModels.PermConversationsUpdateTags,
	ActionSetTags:         authzModels.PermConversationsUpdateTags,
	ActionRemoveTags:      authzModels.PermConversationsUpdateTags,
}

// RuleRecord represents a rule record in the database
type RuleRecord struct {
	ID            int             `db:"id" json:"id"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
	Name          string          `db:"name" json:"name"`
	Description   string          `db:"description" json:"description"`
	Type          string          `db:"type" json:"type"`
	Events        pq.StringArray  `db:"events" json:"events"`
	Enabled       bool            `db:"enabled" json:"enabled"`
	Weight        int             `db:"weight" json:"weight"`
	ExecutionMode string          `db:"execution_mode" json:"execution_mode"`
	Rules         json.RawMessage `db:"rules" json:"rules"`
}

// Rule is the parsed form of a RuleRecord: its trigger type, events, detection criteria,
// and actions.
type Rule struct {
	RuleID        int                 `json:"-"`
	Type          string              `json:"type"`
	ExecutionMode string              `json:"execution_mode"`
	Events        []string            `json:"event"`
	Detection     detection.Detection `json:"detection"`
	Actions       []RuleAction        `json:"actions"`
}

// RuleAction is a single action applied when a rule matches; DisplayValue is UI-only
// (not persisted).
type RuleAction struct {
	Type         string   `json:"type" db:"type"`
	Value        []string `json:"value" db:"value"`
	DisplayValue []string `json:"display_value" db:"-"`
}
