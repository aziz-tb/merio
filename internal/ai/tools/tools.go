// Package aitools provides CRUD access to the ai_tools configuration
// table, which holds only Python custom tools; internal capabilities
// (kb_search, etc.) are registered in Go via internal/ai/tools/builtin/.
package aitools

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"

	"github.com/aziz-tb/merio/internal/ai/tools/models"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Manager is the DB-backed CRUD façade for ai_tools.
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
	GetAll    *sqlx.Stmt `query:"get-all"`
	CountAll  *sqlx.Stmt `query:"count-all"`
	GetByID   *sqlx.Stmt `query:"get-by-id"`
	GetBySlug *sqlx.Stmt `query:"get-by-slug"`
	GetByIDs  *sqlx.Stmt `query:"get-by-ids"`
	Insert    *sqlx.Stmt `query:"insert"`
	Update    *sqlx.Stmt `query:"update"`
	Delete    *sqlx.Stmt `query:"delete"`
}

// New builds a Manager.
func New(opt Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opt.Lo, i18n: opt.I18n}, nil
}

// List returns tools with optional enabled filter and pagination. Each
// row's Implementation is reconstructed from its linked functions row.
func (m *Manager) List(enabled *bool, limit, offset int) ([]models.Tool, int, error) {
	out := make([]models.Tool, 0)
	if err := m.q.GetAll.Select(&out, enabled, limit, offset); err != nil {
		m.lo.Error("error listing ai tools", "error", err)
		return out, 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	for i := range out {
		buildImplementation(&out[i])
	}
	var total int
	if err := m.q.CountAll.Get(&total, enabled); err != nil {
		m.lo.Error("error counting ai tools", "error", err)
		return out, 0, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, total, nil
}

// Get returns one tool by ID, with Implementation reconstructed from its
// linked functions row.
func (m *Manager) Get(id int) (models.Tool, error) {
	var t models.Tool
	if err := m.q.GetByID.Get(&t, id); err != nil {
		if err == sql.ErrNoRows {
			return t, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error fetching ai tool", "id", id, "error", err)
		return t, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	buildImplementation(&t)
	return t, nil
}

// getBySlug returns one tool by its slug. Used internally by the
// dispatcher; not exposed via the HTTP CRUD surface (the public list/get
// endpoints use ID).
func (m *Manager) getBySlug(slug string) (models.Tool, error) {
	var t models.Tool
	if err := m.q.GetBySlug.Get(&t, slug); err != nil {
		if err == sql.ErrNoRows {
			return t, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error fetching ai tool by slug", "slug", slug, "error", err)
		return t, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	buildImplementation(&t)
	return t, nil
}

// GetByIDs returns tools whose ID is in the provided list. Used by the
// Agent handler to validate that every tools[].tool_id reference points
// to an existing ai_tools row.
func (m *Manager) GetByIDs(ids []int) ([]models.Tool, error) {
	out := make([]models.Tool, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	intArr := make(pq.Int64Array, len(ids))
	for i, id := range ids {
		intArr[i] = int64(id)
	}
	if err := m.q.GetByIDs.Select(&out, intArr); err != nil {
		m.lo.Error("error fetching ai tools by ids", "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	for i := range out {
		buildImplementation(&out[i])
	}
	return out, nil
}

// Create inserts a new tool row pointing at an already-created functions
// row (t.FunctionID). Caller validates JSONB shapes/slug and owns the
// functions row. Implementation is not stored here — it lives on functions.
func (m *Manager) Create(t models.Tool) (models.Tool, error) {
	defaults(&t)
	var out models.Tool
	if err := m.q.Insert.Get(&out,
		t.Name, t.Slug, t.Description,
		t.InputParameters, t.OutputSchema, t.Category,
		t.TimeoutMS, t.Async, t.Enabled, t.FunctionID,
	); err != nil {
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI tool with this name or slug already exists", nil)
		}
		m.lo.Error("error creating ai tool", "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// CreateTx inserts a tool row inside tx.
func (m *Manager) CreateTx(ctx context.Context, tx *sqlx.Tx, t models.Tool) (models.Tool, error) {
	defaults(&t)
	var out models.Tool
	stmt := tx.StmtxContext(ctx, m.q.Insert)
	if err := stmt.GetContext(ctx, &out,
		t.Name, t.Slug, t.Description,
		t.InputParameters, t.OutputSchema, t.Category,
		t.TimeoutMS, t.Async, t.Enabled, t.FunctionID,
	); err != nil {
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI tool with this name or slug already exists", nil)
		}
		m.lo.Error("error creating ai tool", "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Update overwrites all mutable fields and bumps updated_at.
func (m *Manager) Update(id int, t models.Tool) (models.Tool, error) {
	defaults(&t)
	var out models.Tool
	if err := m.q.Update.Get(&out, id,
		t.Name, t.Slug, t.Description,
		t.InputParameters, t.OutputSchema, t.Category,
		t.TimeoutMS, t.Async, t.Enabled, t.FunctionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return out, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI tool with this name or slug already exists", nil)
		}
		m.lo.Error("error updating ai tool", "id", id, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// UpdateTx overwrites a tool row inside tx.
func (m *Manager) UpdateTx(ctx context.Context, tx *sqlx.Tx, id int, t models.Tool) (models.Tool, error) {
	defaults(&t)
	var out models.Tool
	stmt := tx.StmtxContext(ctx, m.q.Update)
	if err := stmt.GetContext(ctx, &out, id,
		t.Name, t.Slug, t.Description,
		t.InputParameters, t.OutputSchema, t.Category,
		t.TimeoutMS, t.Async, t.Enabled, t.FunctionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return out, envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
		}
		if isUniqueViolation(err) {
			return out, envelope.NewError(envelope.InputError, "AI tool with this name or slug already exists", nil)
		}
		m.lo.Error("error updating ai tool", "id", id, "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// Delete hard-deletes a tool by ID.
func (m *Manager) Delete(id int) error {
	res, err := m.q.Delete.Exec(id)
	if err != nil {
		m.lo.Error("error deleting ai tool", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
	}
	return nil
}

// DeleteTx hard-deletes a tool row inside tx.
func (m *Manager) DeleteTx(ctx context.Context, tx *sqlx.Tx, id int) error {
	stmt := tx.StmtxContext(ctx, m.q.Delete)
	res, err := stmt.ExecContext(ctx, id)
	if err != nil {
		m.lo.Error("error deleting ai tool", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return envelope.NewError(envelope.InputError, m.i18n.T("globals.messages.notFound"), nil)
	}
	return nil
}

// defaults fills zero-valued fields so INSERT/UPDATE never sends NULL for
// the JSONB NOT NULL columns.
func defaults(t *models.Tool) {
	if len(t.InputParameters) == 0 {
		t.InputParameters = json.RawMessage(`[]`)
	}
	if len(t.OutputSchema) == 0 {
		t.OutputSchema = json.RawMessage(`{}`)
	}
	if t.TimeoutMS == 0 {
		t.TimeoutMS = 30000
	}
	if t.Category == "" {
		t.Category = "write"
	}
}

// buildImplementation reconstructs the {code, language, packages} JSON view
// from the JOINed functions columns for the admin API response.
func buildImplementation(t *models.Tool) {
	b, _ := json.Marshal(models.PythonScriptImpl{
		Code:     t.FnCode,
		Language: t.FnLanguage,
		Packages: t.FnPackages,
	})
	t.Implementation = b
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// failure (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
		return true
	}
	return false
}
