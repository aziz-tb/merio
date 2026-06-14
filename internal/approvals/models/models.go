// Package models holds DB-wire types for agent_write_approvals — the durable
// record of a proposed write tool call that requires human approval before it
// executes (Slice 4, run-ends-at-write). The record is the contract between the
// run that stopped at the write and the run that resumes after approval; the
// Slice 8 fast-follow reuses it unchanged (only the resume mechanism — run
// restart vs. in-workflow signal — differs).
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Approval statuses. A record starts pending; resolving it flips to exactly
// one terminal status (the atomic transition is the idempotency lock).
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusEdited   = "edited"
	StatusRejected = "rejected"
)

// WriteApproval mirrors an agent_write_approvals row.
//
// References use FK ids (agent_id, tool_id) per the ids-in-table convention;
// the gate and the LLM tool call stay slug-driven. GatedRunTraceID is the run
// this approval gates (the blocked run — v1 reads its trace to resume; Slice 8
// signals its still-paused workflow). ParentTraceID is the explicit composer
// run a sub-agent gate belongs to (NULL when the composer itself gated), so
// resume never depends on parsing the ":sub:" trace-id encoding.
type WriteApproval struct {
	ID               int64           `db:"id" json:"id"`
	ConversationID   int64           `db:"conversation_id" json:"conversation_id"`
	GatedRunTraceID  string          `db:"gated_run_trace_id" json:"gated_run_trace_id"`
	ParentTraceID    null.String     `db:"parent_trace_id" json:"parent_trace_id"`
	AgentID          null.Int        `db:"agent_id" json:"agent_id"`
	ToolID           int             `db:"tool_id" json:"tool_id"`
	ToolUseID        string          `db:"tool_use_id" json:"tool_use_id"`
	ProposedInputs   json.RawMessage `db:"proposed_inputs" json:"proposed_inputs"`
	Status           string          `db:"status" json:"status"`
	ApprovedInputs   json.RawMessage `db:"approved_inputs" json:"approved_inputs"`
	RejectionReason  null.String     `db:"rejection_reason" json:"rejection_reason"`
	ReviewedByUserID null.Int64      `db:"reviewed_by_user_id" json:"reviewed_by_user_id"`
	ResolvedAt       null.Time       `db:"resolved_at" json:"resolved_at"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}
