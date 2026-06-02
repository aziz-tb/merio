// Package aiagents provides CRUD access to the ai_agents configuration
// table (F9). Agents are the narrative LLM-with-tools loops authored by
// admins; this package owns persistence only — runtime evaluation lives
// elsewhere in the future pipeline package.
package aiagents

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"

	"github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

// emptyKBScope is the canonical zero-value for the kb_scope JSONB
// column — handed back to Postgres when the caller leaves the field
// blank so the DB never sees an empty JSON document.
var emptyKBScope = json.RawMessage(`{"folder_ids":[],"article_ids":[]}`)

// DefaultDetection is the scaffold stored for agents that do not yet have
// routing rules. It keeps the admin detection builder usable on first edit.
const DefaultDetection = `{"mode":"manual_only","rules":{"op":"AND","children":[]},"ai":{"prompt":""}}`

//go:embed queries.sql
var efs embed.FS

// Manager is the DB-backed CRUD façade for ai_agents.
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
	GetAll   *sqlx.Stmt `query:"get-all"`
	CountAll *sqlx.Stmt `query:"count-all"`
	GetByID  *sqlx.Stmt `query:"get-by-id"`
	Insert   *sqlx.Stmt `query:"insert"`
	Update   *sqlx.Stmt `query:"update"`
	Delete   *sqlx.Stmt `query:"delete"`
}

// New builds a Manager.
func New(opt Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opt.Lo, i18n: opt.I18n}, nil
}

// List returns agents with optional enabled filter and pagination.
// Pass enabled=nil to return both. Limit/offset must be > 0 for limit,
// >= 0 for offset; the handler layer applies sane defaults.
func (m *Manager) List(enabled *bool, limit, offset int) ([]models.Agent, int, error) {
	out := make([]models.Agent, 0)
	if err := m.q.GetAll.Select(&out, enabled, limit, offset); err != nil {
		m.lo.Error("error listing ai agents", "error", err)
		return out, 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	var total int
	if err := m.q.CountAll.Get(&total, enabled); err != nil {
		m.lo.Error("error counting ai agents", "error", err)
		return out, 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, total, nil
}

// ListEnabled returns enabled agents for runtime validation.
func (m *Manager) ListEnabled(ctx context.Context) ([]models.Agent, error) {
	enabled := true
	out := make([]models.Agent, 0)
	if err := m.q.GetAll.SelectContext(ctx, &out, &enabled, 1000, 0); err != nil {
		m.lo.Error("error listing enabled ai agents", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Get returns one agent by ID.
func (m *Manager) Get(id int) (models.Agent, error) {
	var a models.Agent
	if err := m.q.GetByID.Get(&a, id); err != nil {
		if err == sql.ErrNoRows {
			return a, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error fetching ai agent", "id", id, "error", err)
		return a, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return a, nil
}

// Create inserts a new agent. Caller is responsible for validating
// JSONB shapes, slug format, kb_scope folder/article existence, and tool refs.
func (m *Manager) Create(a models.Agent) (models.Agent, error) {
	defaults(&a)
	var out models.Agent
	if err := m.q.Insert.Get(&out,
		a.Name, a.Slug, a.Description, a.Prompt,
		a.Detection, a.Tools, a.KBScope,
		a.PresetOverrides, a.ConfidenceThreshold, a.MaxIterations, a.ExtendedThinkingEnabled,
		a.MinVerification, a.Autonomy, a.Enabled,
	); err != nil {
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI agent with this name or slug already exists", nil)
		}
		m.lo.Error("error creating ai agent", "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Update overwrites all mutable fields on an existing agent and bumps
// updated_at. Caller validates the same shape rules as Create.
func (m *Manager) Update(id int, a models.Agent) (models.Agent, error) {
	defaults(&a)
	var out models.Agent
	if err := m.q.Update.Get(&out, id,
		a.Name, a.Slug, a.Description, a.Prompt,
		a.Detection, a.Tools, a.KBScope,
		a.PresetOverrides, a.ConfidenceThreshold, a.MaxIterations, a.ExtendedThinkingEnabled,
		a.MinVerification, a.Autonomy, a.Enabled,
	); err != nil {
		if err == sql.ErrNoRows {
			return out, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI agent with this name or slug already exists", nil)
		}
		m.lo.Error("error updating ai agent", "id", id, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Delete hard-deletes an agent by ID.
func (m *Manager) Delete(id int) error {
	res, err := m.q.Delete.Exec(id)
	if err != nil {
		m.lo.Error("error deleting ai agent", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
	}
	return nil
}

// defaults fills NULL-equivalent fields with their canonical zero values
// so the INSERT/UPDATE never sends Postgres NULL for columns that are
// JSONB NOT NULL or expected non-empty strings.
func defaults(a *models.Agent) {
	if len(a.Detection) == 0 || string(a.Detection) == "{}" || string(a.Detection) == "null" {
		a.Detection = json.RawMessage(DefaultDetection)
	}
	if len(a.Tools) == 0 {
		a.Tools = json.RawMessage(`[]`)
	}
	if len(a.Autonomy) == 0 {
		a.Autonomy = json.RawMessage(`{}`)
	}
	if len(a.KBScope) == 0 {
		a.KBScope = emptyKBScope
	}
	if len(a.PresetOverrides) == 0 {
		a.PresetOverrides = json.RawMessage(`{}`)
	}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// failure (SQLSTATE 23505) — used to translate slug/name collisions
// into 400-class envelope errors instead of 500s.
func isUniqueViolation(err error) bool {
	if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
		return true
	}
	return false
}
