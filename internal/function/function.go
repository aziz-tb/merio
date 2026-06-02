// Package function manages the functions table — the single, unified home
// for every named, sandbox-runnable code blob across the platform (custom
// AI tools, customer-identification executors, and future domains). It
// loads a function by name and runs it through the F6 exec runner; callers
// (the AI tool dispatcher, the customer-identification workflow activities,
// the admin preview endpoint) never talk to the sandbox directly.
package function

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"strings"

	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/abhinavxd/libredesk/internal/envelope"
	execModels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
	"github.com/abhinavxd/libredesk/internal/function/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// Typed errors so callers (dispatcher, HTTP handlers, workflow
	// activities) can branch on the failure without string matching.
	ErrFunctionNotFound  = errors.New("function: not found")
	ErrFunctionCodeEmpty = errors.New("function: code is empty")
	ErrFunctionProtected = errors.New("function: row is protected and cannot be deleted")
)

// Runner is the slice of the F6 exec runner the package needs. Defined as
// an interface so tests can substitute a fake without a live sandbox.
type Runner interface {
	Execute(ctx context.Context, req execModels.Request) (*execModels.Result, error)
}

// Manager is the DB-backed façade for the functions table.
type Manager struct {
	q      queries
	runner Runner
	lo     *logf.Logger
	i18n   *i18n.I18n
}

// Opts wires dependencies into the Manager.
type Opts struct {
	DB     *sqlx.DB
	Runner Runner
	Lo     *logf.Logger
	I18n   *i18n.I18n
}

type queries struct {
	GetByName    *sqlx.Stmt `query:"get-function-by-name"`
	GetByID      *sqlx.Stmt `query:"get-function-by-id"`
	ListByDomain *sqlx.Stmt `query:"list-functions-by-domain"`
	Insert       *sqlx.Stmt `query:"insert-function"`
	Update       *sqlx.Stmt `query:"update-function-by-name"`
	Delete       *sqlx.Stmt `query:"delete-function-by-name"`
}

// New creates and returns a new Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:      q,
		runner: opts.Runner,
		lo:     opts.Lo,
		i18n:   opts.I18n,
	}, nil
}

// GetByName returns one function by its globally-unique name.
func (m *Manager) GetByName(name string) (*models.Function, error) {
	var f models.Function
	if err := m.q.GetByName.Get(&f, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFunctionNotFound
		}
		m.lo.Error("error fetching function by name", "name", name, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return &f, nil
}

// GetByID returns one function by internal id. Used by callers that hold a
// function_id FK (e.g. ai_tools).
func (m *Manager) GetByID(id int64) (*models.Function, error) {
	var f models.Function
	if err := m.q.GetByID.Get(&f, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrFunctionNotFound
		}
		m.lo.Error("error fetching function by id", "id", id, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return &f, nil
}

// ListByDomain returns every function in a domain, or all functions when
// domain is empty.
func (m *Manager) ListByDomain(domain string) ([]models.Function, error) {
	out := make([]models.Function, 0)
	if err := m.q.ListByDomain.Select(&out, domain); err != nil {
		m.lo.Error("error listing functions by domain", "domain", domain, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Create inserts a new function row and returns its id. Go-only — the
// ai_tools admin handler calls this for transparent dual-write.
func (m *Manager) Create(f *models.Function) (int64, error) {
	defaults(f)
	var id int64
	if err := m.q.Insert.Get(&id,
		f.Name, f.Domain, f.Code, f.Language, f.Packages, f.TimeoutMS, f.Parameters, f.UpdatedByUserID,
	); err != nil {
		m.lo.Error("error creating function", "name", f.Name, "error", err)
		return 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return id, nil
}

// CreateTx inserts a function row inside tx and returns its id.
func (m *Manager) CreateTx(ctx context.Context, tx *sqlx.Tx, f *models.Function) (int64, error) {
	defaults(f)
	var id int64
	stmt := tx.StmtxContext(ctx, m.q.Insert)
	if err := stmt.GetContext(ctx, &id,
		f.Name, f.Domain, f.Code, f.Language, f.Packages, f.TimeoutMS, f.Parameters, f.UpdatedByUserID,
	); err != nil {
		m.lo.Error("error creating function", "name", f.Name, "error", err)
		return 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	f.ID = id
	return id, nil
}

// Update overwrites the mutable fields of the function identified by name.
// Name and domain are immutable and cannot be changed here.
func (m *Manager) Update(name string, f *models.Function) error {
	defaults(f)
	res, err := m.q.Update.Exec(
		name, f.Code, f.Language, f.Packages, f.TimeoutMS, f.Parameters, f.UpdatedByUserID,
	)
	if err != nil {
		m.lo.Error("error updating function", "name", name, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFunctionNotFound
	}
	return nil
}

// UpdateTx overwrites mutable fields inside tx.
func (m *Manager) UpdateTx(ctx context.Context, tx *sqlx.Tx, name string, f *models.Function) error {
	defaults(f)
	stmt := tx.StmtxContext(ctx, m.q.Update)
	res, err := stmt.ExecContext(ctx,
		name, f.Code, f.Language, f.Packages, f.TimeoutMS, f.Parameters, f.UpdatedByUserID,
	)
	if err != nil {
		m.lo.Error("error updating function", "name", name, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFunctionNotFound
	}
	return nil
}

// Delete hard-deletes a function by name. Go-only. Refuses to delete rows
// that aren't custom AI tools — customer-identification (and future seeded)
// rows are protected enum members, not runtime-deletable.
func (m *Manager) Delete(name string) error {
	f, err := m.GetByName(name)
	if err != nil {
		return err
	}
	if f.Domain != models.DomainAITool {
		return ErrFunctionProtected
	}
	if _, err := m.q.Delete.Exec(name); err != nil {
		m.lo.Error("error deleting function", "name", name, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// DeleteTx hard-deletes an AI-tool function inside tx.
func (m *Manager) DeleteTx(ctx context.Context, tx *sqlx.Tx, name string) error {
	var f models.Function
	getStmt := tx.StmtxContext(ctx, m.q.GetByName)
	if err := getStmt.GetContext(ctx, &f, name); err != nil {
		if err == sql.ErrNoRows {
			return ErrFunctionNotFound
		}
		m.lo.Error("error fetching function by name", "name", name, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	if f.Domain != models.DomainAITool {
		return ErrFunctionProtected
	}
	delStmt := tx.StmtxContext(ctx, m.q.Delete)
	if _, err := delStmt.ExecContext(ctx, name); err != nil {
		m.lo.Error("error deleting function", "name", name, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// RunRequest is the input to Run.
type RunRequest struct {
	Name           string         // the function to run
	Input          map[string]any // becomes sdk.input inside the sandbox
	InvokerUserID  int            // operator user id, or 0 for system runs (runner substitutes System Agent)
	ConversationID int            // optional; 0 = none
	TraceID        string         // correlates audit rows; runner generates one when empty
}

// Run loads a function by name and executes it through the F6 sandbox. The
// runner writes the function_executions audit row automatically. Script
// outcomes (failure/error/noop) come back as a non-nil Result with the
// matching Kind, NOT as a Go error; infra/typed failures return an error.
func (m *Manager) Run(ctx context.Context, req RunRequest) (*execModels.Result, error) {
	f, err := m.GetByName(req.Name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(f.Code) == "" {
		return nil, ErrFunctionCodeEmpty
	}

	timeoutSec := f.TimeoutMS / 1000
	if timeoutSec <= 0 && f.TimeoutMS > 0 {
		timeoutSec = 1
	}

	rReq := execModels.Request{
		InvokerUserID:  req.InvokerUserID,
		Code:           f.Code,
		Language:       f.Language,
		Packages:       SplitPackages(f.Packages),
		TimeoutSeconds: timeoutSec,
		Input:          req.Input,
		ToolName:       f.Name,
		ToolDepth:      0,
		TraceID:        req.TraceID,
	}
	if req.ConversationID > 0 {
		rReq.ConversationID = &req.ConversationID
	}
	return m.runner.Execute(ctx, rReq)
}

// defaults fills columns whose zero value would violate a NOT NULL / CHECK
// constraint when the caller left them unset.
func defaults(f *models.Function) {
	if len(f.Parameters) == 0 {
		f.Parameters = json.RawMessage(`[]`)
	}
	if f.TimeoutMS == 0 {
		f.TimeoutMS = 30000
	}
	if strings.TrimSpace(f.Language) == "" {
		f.Language = "python"
	}
}

// SplitPackages breaks a packages string (newline-, whitespace-, or
// comma-separated pip/npm requirements) into the slice the runner accepts.
// Empty entries are dropped.
func SplitPackages(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ','
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
