// Package worklog records ticket-touching actions (write/status/note/forward)
// for analytics + reporting. Manager.StartWorklog is paired with EndWorklog,
// which stamps the end time and computes duration at the DB level. See
// internal/conversation for the instrumentation hooks.
package worklog

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"

	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/aziz-tb/merio/internal/worklog/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Manager records and reads worklog entries.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

type queries struct {
	Start             *sqlx.Stmt `query:"start"`
	StartByUUID       *sqlx.Stmt `query:"start-by-uuid"`
	End               *sqlx.Stmt `query:"end"`
	GetByConversation *sqlx.Stmt `query:"get-by-conversation"`
	GetByUser         *sqlx.Stmt `query:"get-by-user"`
}

// Opts contains the dependencies for the worklog manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
}

// New initializes a worklog manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// StartWorklog inserts a row with started_at = NOW() and returns its id.
// userID may be nil for system-initiated actions. contextPayload may be nil.
// ctx is honored for cancellation.
func (m *Manager) StartWorklog(
	ctx context.Context,
	convID int64,
	userID *int64,
	actionType string,
	contextPayload json.RawMessage,
) (int64, error) {
	var id int64
	if err := m.q.Start.GetContext(ctx, &id, convID, userID, actionType, nullableJSON(contextPayload)); err != nil {
		m.lo.Error("error starting worklog", "error", err, "action_type", actionType, "conversation_id", convID)
		return 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return id, nil
}

// StartWorklogByUUID is StartWorklog using a conversation UUID. Returns
// (0, sql.ErrNoRows) when the UUID doesn't resolve — call sites treat that
// as a silent skip rather than propagating it.
func (m *Manager) StartWorklogByUUID(
	ctx context.Context,
	convUUID string,
	userID *int64,
	actionType string,
	contextPayload json.RawMessage,
) (int64, error) {
	var id int64
	err := m.q.StartByUUID.GetContext(ctx, &id, convUUID, userID, actionType, nullableJSON(contextPayload))
	if err == sql.ErrNoRows {
		return 0, sql.ErrNoRows
	}
	if err != nil {
		m.lo.Error("error starting worklog by uuid", "error", err, "action_type", actionType, "conversation_uuid", convUUID)
		return 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return id, nil
}

// EndWorklog stamps ended_at = NOW(), computes duration_seconds at the DB,
// and shallow-merges `extra` into the existing context jsonb. Calling End
// twice on the same id is a silent no-op (the UPDATE has AND ended_at IS NULL).
func (m *Manager) EndWorklog(
	ctx context.Context,
	worklogID int64,
	extra json.RawMessage,
) error {
	if worklogID == 0 {
		return nil
	}
	if _, err := m.q.End.ExecContext(ctx, worklogID, nullableJSON(extra)); err != nil {
		m.lo.Error("error ending worklog", "error", err, "worklog_id", worklogID)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// GetWorklogsByConversation lists worklogs for one conversation, newest first.
func (m *Manager) GetWorklogsByConversation(
	ctx context.Context,
	convID int64,
	opts models.ListOpts,
) ([]models.Worklog, error) {
	out := make([]models.Worklog, 0)
	if err := m.q.GetByConversation.SelectContext(ctx, &out, convID, opts.Limit(), opts.Offset()); err != nil {
		m.lo.Error("error fetching worklogs by conversation", "error", err, "conversation_id", convID)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetWorklogsByUser lists worklogs for one user, newest first.
func (m *Manager) GetWorklogsByUser(
	ctx context.Context,
	userID int64,
	opts models.ListOpts,
) ([]models.Worklog, error) {
	out := make([]models.Worklog, 0)
	if err := m.q.GetByUser.SelectContext(ctx, &out, userID, opts.Limit(), opts.Offset()); err != nil {
		m.lo.Error("error fetching worklogs by user", "error", err, "user_id", userID)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// nullableJSON returns nil for an empty payload so the SQL parameter binds as
// SQL NULL rather than an empty bytea — keeps the jsonb || jsonb merge safe.
func nullableJSON(b json.RawMessage) interface{} {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
