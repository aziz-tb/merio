// Package executions is the persistence layer for the Composer tab —
// agent_executions (immutable per-run records) and
// agent_execution_outcomes (at-most-one commitment per run).
//
// The runner stays stateless; this package is the boundary where its
// output materializes in Postgres. Future pipeline-driven runs use the
// same Insert path with source="pipeline" + run_by_user_id=NULL.
package executions

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"strings"

	"github.com/aziz-tb/merio/internal/ai/executions/models"
	"github.com/aziz-tb/merio/internal/ai/runner"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	wfairuns "github.com/aziz-tb/merio/internal/workflows/airuns"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Sentinel errors. Mapped to HTTP codes by the handler.
var (
	// ErrOutcomeExists fires when a second InsertOutcome attempt hits
	// the UNIQUE(agent_execution_id) constraint. Idempotency response:
	// 409 with the existing outcome in the body.
	ErrOutcomeExists = errors.New("executions: outcome already recorded for this run")

	// ErrNotFound fires when GetByID or GetOutcomeByExecutionID find no
	// row. Translates to 404 at the handler.
	ErrNotFound = errors.New("executions: row not found")
)

// Manager is the DB-backed CRUD façade for the two tables.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Opts wires dependencies.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

type queries struct {
	InsertExecution         *sqlx.Stmt `query:"insert-execution"`
	GetExecutionByID        *sqlx.Stmt `query:"get-execution-by-id"`
	LatestByTrace           *sqlx.Stmt `query:"get-latest-execution-by-trace"`
	LatestForConversation   *sqlx.Stmt `query:"latest-executions-by-conversation"`
	LatestExecutionSingle   *sqlx.Stmt `query:"latest-execution-for-conversation"`
	ListByAgent             *sqlx.Stmt `query:"list-by-agent"`
	InsertOutcome           *sqlx.Stmt `query:"insert-outcome"`
	GetOutcomeByExecutionID *sqlx.Stmt `query:"get-outcome-by-execution"`
}

// New builds a Manager with prepared statements.
func New(opt Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opt.Lo, i18n: opt.I18n}, nil
}

// InsertParams carries everything the handler captures from a Run()
// response. Trace and Usage are pre-serialized JSON so this package
// doesn't take a dependency on the runner's structured types.
type InsertParams struct {
	ConversationID      int64
	AgentID             int
	Source              string
	ResolutionAttemptID *int
	TraceID             string
	Status              string
	Draft               *string
	Trace               json.RawMessage
	Usage               json.RawMessage
	DurationMs          int
	RunByUserID         *int64
	AgentUpdatedAt      interface{} // time.Time; interface{} to keep the package import-light
}

// InsertExecution writes one agent_executions row and returns the
// inserted row (with its DB-generated id + run_at + timestamps).
func (m *Manager) InsertExecution(p InsertParams) (models.AgentExecution, error) {
	if len(p.Trace) == 0 {
		p.Trace = json.RawMessage(`[]`)
	}
	if len(p.Usage) == 0 {
		p.Usage = json.RawMessage(`{}`)
	}
	var resAttempt interface{}
	if p.ResolutionAttemptID != nil {
		resAttempt = *p.ResolutionAttemptID
	}
	var runBy interface{}
	if p.RunByUserID != nil {
		runBy = *p.RunByUserID
	}
	var draft interface{}
	if p.Draft != nil {
		draft = *p.Draft
	}
	var out models.AgentExecution
	if err := m.q.InsertExecution.Get(&out,
		p.ConversationID, p.AgentID, p.Source, resAttempt,
		p.TraceID, p.Status, draft, p.Trace, p.Usage, p.DurationMs,
		runBy, p.AgentUpdatedAt,
	); err != nil {
		m.lo.Error("error inserting agent_execution", "agent_id", p.AgentID, "conversation_id", p.ConversationID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetByTraceID loads the most recent execution row for a trace id. Used by the
// approval resume to reconstruct context from the gated run's persisted trace.
// Returns ErrNotFound when no run exists for the trace.
func (m *Manager) GetByTraceID(traceID string) (models.AgentExecution, error) {
	var out models.AgentExecution
	if err := m.q.LatestByTrace.Get(&out, traceID); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching agent_execution by trace", "trace_id", traceID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetByID loads one execution row. Returns ErrNotFound when missing.
func (m *Manager) GetByID(id int64) (models.AgentExecution, error) {
	var out models.AgentExecution
	if err := m.q.GetExecutionByID.Get(&out, id); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching agent_execution", "id", id, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// LatestExecutionForConversation returns the single newest composer
// execution for a conversation. Used by the async run-state endpoint to
// reconnect after completion.
func (m *Manager) LatestExecutionForConversation(conversationID int64) (models.AgentExecution, error) {
	var out models.AgentExecution
	if err := m.q.LatestExecutionSingle.Get(&out, conversationID); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching latest agent_execution for conversation",
			"conversation_id", conversationID,
			"error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// LatestForConversation returns the latest composer-source execution
// per enabled agent for the given conversation_id, with the outcome
// joined (nil when none) and is_stale computed from the agent's
// current updated_at vs. the snapshot taken at run time.
//
// The supporting SQL query joins the three tables and aliases columns
// with dotted names; we scan into a flat struct and then re-shape into
// the nested API response. sqlx's named-mapper handles the dotted
// aliases when we use Queryx + StructScan into a flat target.
func (m *Manager) LatestForConversation(conversationID int64) ([]models.LatestExecutionRow, error) {
	rows, err := m.q.LatestForConversation.Queryx(conversationID)
	if err != nil {
		m.lo.Error("error querying latest executions", "conversation_id", conversationID, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	defer rows.Close()

	out := make([]models.LatestExecutionRow, 0)
	for rows.Next() {
		var flat latestExecutionFlat
		if err := rows.StructScan(&flat); err != nil {
			m.lo.Error("error scanning latest-executions row", "error", err)
			return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
		}

		row := models.LatestExecutionRow{
			Execution: flat.Execution,
			Agent:     flat.Agent,
			IsStale:   flat.Agent.UpdatedAt.After(flat.Execution.AgentUpdatedAt),
		}
		if flat.OutcomeID.Valid {
			row.Outcome = &models.AgentExecutionOutcome{
				ID:                flat.OutcomeID.Int64,
				AgentExecutionID:  flat.OutcomeAgentExecutionID.Int64,
				Outcome:           flat.OutcomeKind.String,
				OutcomeReason:     flat.OutcomeReason,
				MessageID:         flat.OutcomeMessageID,
				CommittedByUserID: flat.OutcomeCommittedBy,
				CommittedAt:       flat.OutcomeCommittedAt.Time,
				CreatedAt:         flat.OutcomeCreatedAt.Time,
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// latestExecutionFlat is the flat row shape we scan into from the
// dotted-alias query. sqlx maps "agent_execution.id" → Execution.ID via
// the dot path. Outcome columns use null.X because the LEFT JOIN
// produces NULLs when an execution has no outcome row yet.
type latestExecutionFlat struct {
	Execution models.AgentExecution `db:"agent_execution"`
	Agent     models.AgentSummary   `db:"agent"`

	OutcomeID               null.Int64  `db:"outcome.id"`
	OutcomeAgentExecutionID null.Int64  `db:"outcome.agent_execution_id"`
	OutcomeKind             null.String `db:"outcome.outcome"`
	OutcomeReason           null.String `db:"outcome.outcome_reason"`
	OutcomeMessageID        null.Int64  `db:"outcome.message_id"`
	OutcomeCommittedBy      null.Int64  `db:"outcome.committed_by_user_id"`
	OutcomeCommittedAt      null.Time   `db:"outcome.committed_at"`
	OutcomeCreatedAt        null.Time   `db:"outcome.created_at"`
}

// ListByAgentParams drives the paginated History tab query. Filters
// are all optional. Cursor: pass (BeforeRunAt, BeforeID) from the
// last row of the previous page to fetch the next page.
type ListByAgentParams struct {
	AgentID     int
	Status      string // "" = no filter
	Search      string // ILIKE on draft; "" = no filter
	BeforeRunAt *string
	BeforeID    *int64
	Limit       int
}

// ListByAgent returns past runs for an agent, newest first. Excludes
// runs whose outcome is 'discarded'.
func (m *Manager) ListByAgent(p ListByAgentParams) ([]models.AgentExecution, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 20
	}
	var (
		beforeRunAt interface{}
		beforeID    interface{}
	)
	if p.BeforeRunAt != nil && p.BeforeID != nil {
		beforeRunAt = *p.BeforeRunAt
		beforeID = *p.BeforeID
	}
	rows, err := m.q.ListByAgent.Queryx(p.AgentID, p.Status, p.Search, beforeRunAt, beforeID, p.Limit)
	if err != nil {
		m.lo.Error("error querying agent history", "agent_id", p.AgentID, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	defer rows.Close()
	out := make([]models.AgentExecution, 0, p.Limit)
	for rows.Next() {
		var e models.AgentExecution
		if err := rows.StructScan(&e); err != nil {
			m.lo.Error("error scanning agent history row", "error", err)
			return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

func (m *Manager) PersistCompleteRun(
	_ context.Context,
	params wfairuns.AgentRunWorkflowParams,
	resp runner.RunResponse,
) (int64, error) {
	if params.ConversationID <= 0 || resp.AgentUpdatedAt.IsZero() {
		return 0, nil
	}

	traceJSON, err := json.Marshal(resp.Trace)
	if err != nil {
		traceJSON = json.RawMessage(`[]`)
	}
	usageJSON, err := json.Marshal(resp.Usage)
	if err != nil {
		usageJSON = json.RawMessage(`{}`)
	}

	var runByUserID *int64
	if params.InvokerUserID > 0 {
		v := int64(params.InvokerUserID)
		runByUserID = &v
	}

	row, err := m.InsertExecution(InsertParams{
		ConversationID: int64(params.ConversationID),
		AgentID:        params.AgentID,
		Source:         params.Source,
		TraceID:        params.TraceID,
		Status:         string(resp.Status),
		Draft:          resp.Draft,
		Trace:          traceJSON,
		Usage:          usageJSON,
		DurationMs:     resp.Metrics.TotalLatencyMs,
		RunByUserID:    runByUserID,
		AgentUpdatedAt: resp.AgentUpdatedAt,
	})
	if err != nil {
		return 0, err
	}

	// A composer run that escalated records the escalated outcome so a
	// human sees it (and why) — the same agent_execution_outcomes mechanism an
	// operator commits through, here platform-committed (committed_by NULL).
	// Best-effort: an outcome write failure must not lose the execution row.
	// ErrOutcomeExists is benign (a prior attempt or operator already committed).
	if op, ok := escalationOutcomeParams(row.ID, resp); ok {
		if _, oerr := m.InsertOutcome(op); oerr != nil && !errors.Is(oerr, ErrOutcomeExists) {
			m.lo.Warn("executions: failed to record escalated outcome",
				"agent_execution_id", row.ID, "trace_id", params.TraceID, "error", oerr)
		}
	}
	return row.ID, nil
}

// escalationOutcomeParams maps an ESCALATE terminal decision to the
// agent_execution_outcomes row that records it for human review. Returns
// ok=false for any run that is not a committed escalation (answers, drafts,
// sub-agent runs, ordinary composer runs) — those leave outcome commitment to
// the operator (or Slice 7 auto-send). Pure, so the mapping is unit-testable
// without a DB.
func escalationOutcomeParams(executionID int64, resp runner.RunResponse) (InsertOutcomeParams, bool) {
	if resp.Decision == nil || resp.Decision.Kind != runner.DecisionKindEscalate {
		return InsertOutcomeParams{}, false
	}
	var reasonPtr *string
	if reason := strings.TrimSpace(resp.Decision.InternalReason); reason != "" {
		reasonPtr = &reason
	}
	return InsertOutcomeParams{
		AgentExecutionID: executionID,
		Outcome:          models.OutcomeEscalated,
		OutcomeReason:    reasonPtr,
		// MessageID / CommittedByUserID nil — platform-committed, no message.
	}, true
}

func (m *Manager) PersistFailedRun(
	_ context.Context,
	params wfairuns.AgentRunWorkflowParams,
	resp runner.RunResponse,
	_ string,
) (int64, error) {
	if params.ConversationID <= 0 || resp.AgentUpdatedAt.IsZero() {
		return 0, nil
	}

	traceJSON, err := json.Marshal(resp.Trace)
	if err != nil {
		traceJSON = json.RawMessage(`[]`)
	}
	usageJSON, err := json.Marshal(resp.Usage)
	if err != nil {
		usageJSON = json.RawMessage(`{}`)
	}

	var runByUserID *int64
	if params.InvokerUserID > 0 {
		v := int64(params.InvokerUserID)
		runByUserID = &v
	}

	status := string(resp.Status)
	if status == "" {
		status = string(runner.StatusFailed)
	}
	row, err := m.InsertExecution(InsertParams{
		ConversationID: int64(params.ConversationID),
		AgentID:        params.AgentID,
		Source:         params.Source,
		TraceID:        params.TraceID,
		Status:         status,
		Draft:          nil,
		Trace:          traceJSON,
		Usage:          usageJSON,
		DurationMs:     resp.Metrics.TotalLatencyMs,
		RunByUserID:    runByUserID,
		AgentUpdatedAt: resp.AgentUpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// InsertOutcomeParams is the payload for InsertOutcome.
type InsertOutcomeParams struct {
	AgentExecutionID  int64
	Outcome           string
	OutcomeReason     *string
	MessageID         *int64
	CommittedByUserID *int64
}

// InsertOutcome writes one agent_execution_outcomes row. Returns
// ErrOutcomeExists when the UNIQUE constraint fires (idempotency).
// Caller maps to HTTP 409 + returns the existing outcome.
func (m *Manager) InsertOutcome(p InsertOutcomeParams) (models.AgentExecutionOutcome, error) {
	var reason interface{}
	if p.OutcomeReason != nil {
		reason = *p.OutcomeReason
	}
	var msg interface{}
	if p.MessageID != nil {
		msg = *p.MessageID
	}
	var committedBy interface{}
	if p.CommittedByUserID != nil {
		committedBy = *p.CommittedByUserID
	}
	var out models.AgentExecutionOutcome
	if err := m.q.InsertOutcome.Get(&out,
		p.AgentExecutionID, p.Outcome, reason, msg, committedBy,
	); err != nil {
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			return out, ErrOutcomeExists
		}
		m.lo.Error("error inserting outcome", "agent_execution_id", p.AgentExecutionID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetOutcomeByExecutionID returns the outcome row for an execution.
// Returns ErrNotFound when none exists (the normal path for runs that
// haven't been committed).
func (m *Manager) GetOutcomeByExecutionID(executionID int64) (models.AgentExecutionOutcome, error) {
	var out models.AgentExecutionOutcome
	if err := m.q.GetOutcomeByExecutionID.Get(&out, executionID); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching outcome", "agent_execution_id", executionID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}
