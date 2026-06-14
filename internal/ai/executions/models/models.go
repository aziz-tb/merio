// Package models holds DB-wire types for the agent-executions tables
// introduced by migration v2_7_6:
//
//   - AgentExecution: immutable record of one Runner.Run() invocation
//     against a conversation. Inserted on success; never updated.
//   - AgentExecutionOutcome: at-most-one commitment per execution
//     ("sent", "drafted", "escalated", "discarded"). UNIQUE on
//     agent_execution_id enforces the at-most-one rule.
//
// The two tables together are the persistence layer for the Composer
// tab — and for future pipeline-driven runs that flip source to
// 'pipeline' and run_by_user_id to NULL.
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

	// SourceSubagent marks a sub-agent run invoked by an orchestrator via
	// call_subagent. Its trace persists alongside the orchestrator run; only
	// the sub-agent's report flows back to the orchestrator.
	SourceSubagent = "subagent"
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
// trace + usage are kept as raw JSON so the runner's structured types
// (TurnTrace, llm.Usage) don't bleed into this package. The handler
// marshals what the runner returns; readers unmarshal back into the
// runner's types where needed.
//
// draft is nullable from format_version=2 onward: incomplete runs that
// never call produce_draft persist draft=NULL as a loud failure mode.
//
// BIGINT columns use null.Int64; the INT-typed resolution_attempt_id
// uses null.Int.
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
// message_id is nullable because not every outcome corresponds to a
// sent message — "drafted" / "escalated" / "discarded" have no
// associated conversation_messages row. message_id and
// committed_by_user_id are both BIGINT in the schema, so null.Int64.
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

// LatestExecutionRow is the shape returned by LatestForConversation —
// one execution plus its outcome (if any) plus the joined agent
// summary plus a computed stale flag. The frontend Composer tab
// consumes this shape directly; the API handler serializes it
// unchanged.
type LatestExecutionRow struct {
	Execution AgentExecution         `json:"agent_execution"`
	Outcome   *AgentExecutionOutcome `json:"outcome"`
	Agent     AgentSummary           `json:"agent"`
	IsStale   bool                   `json:"is_stale"`
}

// AgentSummary is the trimmed ai_agents projection embedded in the
// latest-execution response. The full Agent struct would surface
// prompt / tools / kb_scope to the tab, which doesn't need them and
// shouldn't see them.
type AgentSummary struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Slug      string    `db:"slug" json:"slug"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
