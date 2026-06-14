// Package composer provides CRUD access to the single global Composer
// configuration (the composer_config singleton table). It owns persistence
// only — the pure inbox-scope resolver (resolve.go) and the save-time validator
// (validate.go) live alongside it; runtime application of the config (assembling
// guidance into the Composer's prompt, evaluating conditions against a ticket)
// is Slice 5 and lives in the run path, not here.
package composer

import (
	"database/sql"
	"embed"
	"encoding/json"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Canonical empty sections, used to seed defaults so Postgres never sees an
// empty JSON document on write and a fresh read always decodes cleanly.
var (
	emptyGuidance   = json.RawMessage(`[]`)
	emptyAutomation = json.RawMessage(`{"auto_send":false,"approval_required_tool_slugs":[]}`)
	emptyEscalation = json.RawMessage(`{"rules":[],"guidance":[]}`)
)

// Manager is the DB-backed façade for the singleton composer_config row.
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
	GetConfig *sqlx.Stmt `query:"get-config"`
	Upsert    *sqlx.Stmt `query:"upsert"`
}

// New builds a Manager.
func New(opt Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opt.Lo, i18n: opt.I18n}, nil
}

// GetConfig returns the single global composer config (id=1). When no row
// exists yet (a fresh install before the first save), it returns an
// empty-default config so callers always get a valid, resolvable Config rather
// than a not-found error.
func (m *Manager) GetConfig() (models.ComposerConfig, error) {
	var c models.ComposerConfig
	if err := m.q.GetConfig.Get(&c); err != nil {
		if err == sql.ErrNoRows {
			return defaultConfig(), nil
		}
		m.lo.Error("error fetching composer config", "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// Upsert writes the singleton config (id=1), creating it on first save and
// overwriting it thereafter. Empty sections are normalized to their canonical
// empty shape.
func (m *Manager) Upsert(c models.ComposerConfig) (models.ComposerConfig, error) {
	applyDefaults(&c)
	var out models.ComposerConfig
	if err := m.q.Upsert.Get(&out, c.Guidance, c.Automation, c.Escalation); err != nil {
		m.lo.Error("error upserting composer config", "error", err)
		return out, envelope.NewError(envelope.GeneralError, m.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

func defaultConfig() models.ComposerConfig {
	return models.ComposerConfig{
		ID:         1,
		Guidance:   emptyGuidance,
		Automation: emptyAutomation,
		Escalation: emptyEscalation,
	}
}

func applyDefaults(c *models.ComposerConfig) {
	if len(c.Guidance) == 0 {
		c.Guidance = emptyGuidance
	}
	if len(c.Automation) == 0 {
		c.Automation = emptyAutomation
	}
	if len(c.Escalation) == 0 {
		c.Escalation = emptyEscalation
	}
}
