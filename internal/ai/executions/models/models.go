// Package models holds DB-wire types for the agent-executions tables:
// AgentExecution is an immutable record of one Runner.Run() (inserted on
// success, never updated); AgentExecutionOutcome is its at-most-one
// commitment, enforced by a UNIQUE on agent_execution_id.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Source values for agent_executions.source.
const (
	SourceComposer = "composer"
	SourcePipeline = "pipeline"
)

// Outcome values for agent_execution_outcomes.outcome.
const (
	OutcomeSent      = "sent"
	OutcomeDrafted   = "drafted"
	OutcomeEscalated = "escalated"
	OutcomeDiscarded = "discarded"
)

// AgentExecution mirrors the agent_executions row.
//
// Trace/Usage stay raw JSON so the runner's types (TurnTrace, llm.Usage)
// don't bleed into this package. Draft is nullable from format_version=2
// onward: a run that never calls produce_draft persists draft=NULL as a
// loud failure mode.
type AgentExecution struct {
	ID                  int64           `db:"id" json:"id"`
	ConversationID      int64           `db:"conversation_id" json:"conversation_id"`
	AgentID             int             `db:"agent_id" json:"agent_id"`
	Source              string          `db:"source" json:"source"`
	ResolutionAttemptID null.Int        `db:"resolution_attempt_id" json:"resolution_attempt_id"`
	TraceID             string          `db:"trace_id" json:"trace_id"`
	FormatVersion       string          `db:"format_version" json:"format_version"`
	Status              string          `db:"status" json:"status"`
	Draft               null.String     `db:"draft" json:"draft"`
	Trace               json.RawMessage `db:"trace" json:"trace"`
	Usage               json.RawMessage `db:"usage" json:"usage"`
	DurationMs          int             `db:"duration_ms" json:"duration_ms"`
	RunByUserID         null.Int64      `db:"run_by_user_id" json:"run_by_user_id"`
	RunAt               time.Time       `db:"run_at" json:"run_at"`
	AgentUpdatedAt      time.Time       `db:"agent_updated_at" json:"agent_updated_at"`
	CreatedAt           time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time       `db:"updated_at" json:"updated_at"`
}

// AgentExecutionOutcome mirrors the agent_execution_outcomes row.
//
// MessageID is nullable because only "sent" maps to a conversation
// message; "drafted"/"escalated"/"discarded" have none.
type AgentExecutionOutcome struct {
	ID                int64       `db:"id" json:"id"`
	AgentExecutionID  int64       `db:"agent_execution_id" json:"agent_execution_id"`
	Outcome           string      `db:"outcome" json:"outcome"`
	OutcomeReason     null.String `db:"outcome_reason" json:"outcome_reason"`
	MessageID         null.Int64  `db:"message_id" json:"message_id"`
	CommittedByUserID null.Int64  `db:"committed_by_user_id" json:"committed_by_user_id"`
	CommittedAt       time.Time   `db:"committed_at" json:"committed_at"`
	CreatedAt         time.Time   `db:"created_at" json:"created_at"`
}

// LatestExecutionRow is the shape returned by LatestForConversation: one
// execution, its outcome (if any), the joined agent summary, and a
// computed stale flag.
type LatestExecutionRow struct {
	Execution AgentExecution         `json:"agent_execution"`
	Outcome   *AgentExecutionOutcome `json:"outcome"`
	Agent     AgentSummary           `json:"agent"`
	IsStale   bool                   `json:"is_stale"`
}

// AgentSummary is the trimmed ai_agents projection embedded in the
// latest-execution response; it deliberately omits prompt/tools/kb_scope,
// which the Composer tab must not see.
type AgentSummary struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Slug      string    `db:"slug" json:"slug"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
