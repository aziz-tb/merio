// Package approvals is the persistence layer for agent_write_approvals — the
// pending-write records that gate an approval-required tool behind human
// review (Slice 4). The runner stops at the write (StatusAwaitingApproval) and
// this package records the proposal; an operator action resolves it and a
// resuming run executes (or skips) the write. The atomic status transition in
// Resolve is the idempotency lock that makes the write execute at most once.
package approvals

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"

	"github.com/aziz-tb/merio/internal/approvals/models"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Sentinel errors. Mapped to HTTP codes by the handler.
var (
	// ErrNotFound — no approval row with that id.
	ErrNotFound = errors.New("approvals: record not found")

	// ErrNotPending — the record exists but is no longer pending (already
	// approved/edited/rejected). The atomic Resolve update matched zero rows;
	// a duplicate/concurrent resolve lands here, so the write can't run twice.
	ErrNotPending = errors.New("approvals: record is not pending")
)

// Manager is the DB-backed CRUD façade for agent_write_approvals.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Opts wires dependencies into Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

type queries struct {
	Insert            *sqlx.Stmt `query:"insert"`
	GetByID           *sqlx.Stmt `query:"get-by-id"`
	GetPendingByTrace *sqlx.Stmt `query:"get-pending-by-trace"`
	ListPendingByConv *sqlx.Stmt `query:"list-pending-by-conversation"`
	Resolve           *sqlx.Stmt `query:"resolve"`
}

// New builds a Manager.
func New(opt Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opt.Lo, i18n: opt.I18n}, nil
}

// CreateParams is the pending-approval insert payload, captured by the run that
// gated. ParentTraceID/AgentID are nil when the composer itself gated.
type CreateParams struct {
	ConversationID  int64
	GatedRunTraceID string
	ParentTraceID   *string
	AgentID         *int
	ToolID          int
	ToolUseID       string
	ProposedInputs  json.RawMessage
}

// Create inserts a pending approval record and returns it.
func (m *Manager) Create(p CreateParams) (models.WriteApproval, error) {
	if len(p.ProposedInputs) == 0 {
		p.ProposedInputs = json.RawMessage(`{}`)
	}
	var out models.WriteApproval
	if err := m.q.Insert.Get(&out,
		p.ConversationID, p.GatedRunTraceID, nullStr(p.ParentTraceID), nullInt(p.AgentID),
		p.ToolID, p.ToolUseID, p.ProposedInputs,
	); err != nil {
		if err == sql.ErrNoRows {
			// ON CONFLICT DO NOTHING: a pending record for this gated run
			// already exists (a retry/duplicate). Return it — idempotent, the
			// write is recorded exactly once.
			existing, gErr := m.GetPendingByTrace(p.GatedRunTraceID)
			if gErr != nil {
				return out, gErr
			}
			return existing, nil
		}
		m.lo.Error("error inserting write approval", "trace_id", p.GatedRunTraceID, "tool_id", p.ToolID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetPendingByTrace loads the open approval for a gated run, if any. Returns
// ErrNotFound when none is pending.
func (m *Manager) GetPendingByTrace(gatedRunTraceID string) (models.WriteApproval, error) {
	var out models.WriteApproval
	if err := m.q.GetPendingByTrace.Get(&out, gatedRunTraceID); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching pending write approval", "trace_id", gatedRunTraceID, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetByID loads one approval record. Returns ErrNotFound when missing.
func (m *Manager) GetByID(id int64) (models.WriteApproval, error) {
	var out models.WriteApproval
	if err := m.q.GetByID.Get(&out, id); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}
		m.lo.Error("error fetching write approval", "id", id, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// ListPendingForConversation returns the open approvals for a conversation
// (powers the operator approval surface).
func (m *Manager) ListPendingForConversation(conversationID int64) ([]models.WriteApproval, error) {
	out := make([]models.WriteApproval, 0)
	if err := m.q.ListPendingByConv.Select(&out, conversationID); err != nil {
		m.lo.Error("error listing pending write approvals", "conversation_id", conversationID, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// ResolveParams resolves a pending approval. Status is one of approved / edited
// / rejected. ApprovedInputs carries the inputs the resuming run will execute
// with (proposed on approve, operator-edited on edit, nil on reject).
type ResolveParams struct {
	ID               int64
	Status           string
	ApprovedInputs   json.RawMessage
	RejectionReason  *string
	ReviewedByUserID *int64
}

// Resolve performs the atomic status transition (pending → terminal). It is the
// idempotency lock: the UPDATE only matches a row still in 'pending', so a
// duplicate or concurrent resolve matches zero rows and returns ErrNotPending —
// the caller must NOT start a resuming run in that case, so the write executes
// at most once.
func (m *Manager) Resolve(p ResolveParams) (models.WriteApproval, error) {
	var approved interface{}
	if len(p.ApprovedInputs) > 0 {
		approved = []byte(p.ApprovedInputs)
	}
	var out models.WriteApproval
	if err := m.q.Resolve.Get(&out,
		p.ID, p.Status, approved, nullStr(p.RejectionReason), nullInt64(p.ReviewedByUserID),
	); err != nil {
		if err == sql.ErrNoRows {
			// Either the row is gone or it is no longer pending. Disambiguate
			// for a clearer operator message; both are "can't resolve".
			if _, getErr := m.GetByID(p.ID); errors.Is(getErr, ErrNotFound) {
				return out, ErrNotFound
			}
			return out, ErrNotPending
		}
		m.lo.Error("error resolving write approval", "id", p.ID, "status", p.Status, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

func nullStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func nullInt(i *int) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

func nullInt64(i *int64) interface{} {
	if i == nil {
		return nil
	}
	return *i
}
