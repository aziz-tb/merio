// Package models defines the worklog domain types.
package models

import (
	"encoding/json"
	"time"
)

// Action types stored in worklogs.action_type. Mirrors the CHECK constraint
// on the worklogs table.
const (
	ActionTypeWrite   = "write"
	ActionTypeStatus  = "status"
	ActionTypeNote    = "note"
	ActionTypeForward = "forward"
)

// Automation levels stored in worklogs.automation_level. Provisioned for Future AI work;
// the writer for L1-L5 is not exposed in this PR — every row inserted today defaults to L0.
const (
	AutomationLevelL0 = "L0"
	AutomationLevelL1 = "L1"
	AutomationLevelL2 = "L2"
	AutomationLevelL3 = "L3"
	AutomationLevelL4 = "L4"
	AutomationLevelL5 = "L5"
)

// Worklog is one ticket-touching action recorded for analytics + reporting.
type Worklog struct {
	ID              int64           `db:"id"               json:"id"`
	ConversationID  int64           `db:"conversation_id"  json:"conversation_id"`
	UserID          *int64          `db:"user_id"          json:"user_id"`
	StartedAt       time.Time       `db:"started_at"       json:"started_at"`
	EndedAt         *time.Time      `db:"ended_at"         json:"ended_at"`
	DurationSeconds *int            `db:"duration_seconds" json:"duration_seconds"`
	ActionType      string          `db:"action_type"      json:"action_type"`
	AutomationLevel string          `db:"automation_level" json:"automation_level"`
	Context         json.RawMessage `db:"context"          json:"context"`
	CreatedAt       time.Time       `db:"created_at"       json:"created_at"`
}

// ListOpts paginates list reads. Filters live in purpose-built queries on
// indexed columns; this struct is paging only for now.
type ListOpts struct {
	Page     int
	PageSize int
}

// Defaults return effective values, applying sane minimums.
func (o ListOpts) effectivePage() int {
	if o.Page < 1 {
		return 1
	}
	return o.Page
}

func (o ListOpts) effectivePageSize() int {
	if o.PageSize < 1 {
		return 50
	}
	if o.PageSize > 500 {
		return 500
	}
	return o.PageSize
}

// Limit returns the effective LIMIT for a paged query.
func (o ListOpts) Limit() int { return o.effectivePageSize() }

// Offset returns the effective OFFSET for a paged query.
func (o ListOpts) Offset() int {
	return (o.effectivePage() - 1) * o.effectivePageSize()
}
