// Package rewrite owns reply-box rewrite prompt lookup.
package rewrite

import (
	"database/sql"
	"embed"
	"time"

	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Prompt is a rewrite prompt shown in the reply-box AI menu.
type Prompt struct {
	ID        int       `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Title     string    `db:"title" json:"title"`
	Key       string    `db:"key" json:"key"`
	Content   string    `db:"content" json:"content,omitempty"`
}

// Manager provides ai_prompts access for reply-box rewrite.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
}

// Opts contains options for initializing Manager.
type Opts struct {
	DB   *sqlx.DB
	I18n *i18n.I18n
	Lo   *logf.Logger
}

type queries struct {
	GetPrompt  *sqlx.Stmt `query:"get-prompt"`
	GetPrompts *sqlx.Stmt `query:"get-prompts"`
}

// New creates a new rewrite prompt manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
	}, nil
}

// PromptContent returns the system prompt text for a rewrite prompt key.
func (m *Manager) PromptContent(k string) (string, error) {
	var p Prompt
	if err := m.q.GetPrompt.Get(&p, k); err != nil {
		if err == sql.ErrNoRows {
			m.lo.Error("rewrite prompt not found", "key", k)
			return "", envelope.NewError(envelope.InputError, m.i18n.T("validation.notFoundTemplate"), nil)
		}
		m.lo.Error("error fetching rewrite prompt", "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return p.Content, nil
}

// GetPrompts returns rewrite prompts for the reply-box AI menu.
func (m *Manager) GetPrompts() ([]Prompt, error) {
	prompts := make([]Prompt, 0)
	if err := m.q.GetPrompts.Select(&prompts); err != nil {
		m.lo.Error("error fetching rewrite prompts", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return prompts, nil
}
