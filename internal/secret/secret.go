// Package secret stores operator-managed platform secrets and decrypts them
// for sandboxed executor code. Plaintext values never leave this package
// except through Get/LoadAll for the runner's in-memory SDK bundle.
package secret

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aziz-tb/merio/internal/dbutil"
	secretmodels "github.com/aziz-tb/merio/internal/secret/models"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

const maxValueBytes = 5 * 1024

var (
	//go:embed queries.sql
	efs embed.FS

	ErrSecretNotFound = errors.New("secret not found")
	ErrSecretExists   = errors.New("secret already exists")
	ErrInvalidName    = errors.New("invalid secret name")
	ErrValueTooLarge  = errors.New("secret value exceeds 5 KB")

	nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// AccessContext annotates secret access log lines with the execution that
// caused them. Values are metadata only; plaintext is never logged.
type AccessContext struct {
	FunctionName   string
	ConversationID int
	TraceID        string
}

// Manager handles encrypted secrets stored in Postgres.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18n.I18n
	kek  []byte
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18n.I18n
	KEK  []byte
}

type queries struct {
	List    *sqlx.Stmt `query:"list-secrets"`
	Insert  *sqlx.Stmt `query:"insert-secret"`
	Update  *sqlx.Stmt `query:"update-secret"`
	Delete  *sqlx.Stmt `query:"delete-secret"`
	Get     *sqlx.Stmt `query:"get-secret-value-by-name"`
	LoadAll *sqlx.Stmt `query:"list-secret-values"`
	Touch   *sqlx.Stmt `query:"touch-secret"`
}

type encryptedSecret struct {
	secretmodels.Secret
	EncryptedValue []byte `db:"encrypted_value"`
	Nonce          []byte `db:"nonce"`
}

// New creates and returns a new Manager.
func New(opts Opts) (*Manager, error) {
	if opts.DB == nil {
		return nil, errors.New("secret: nil db")
	}
	if len(opts.KEK) != 32 {
		return nil, ErrKEKInvalid
	}
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
		kek:  append([]byte(nil), opts.KEK...),
	}, nil
}

// ValidName reports whether a secret name satisfies the platform naming
// contract: lowercase alphanumeric + underscore, max 64 chars, starts with a
// letter.
func ValidName(name string) bool {
	return nameRe.MatchString(strings.TrimSpace(name))
}

// MaxValueBytes returns the maximum accepted plaintext byte size.
func MaxValueBytes() int {
	return maxValueBytes
}

// List returns metadata for all secrets. Plaintext is never included.
func (m *Manager) List(ctx context.Context) ([]secretmodels.Secret, error) {
	out := make([]secretmodels.Secret, 0)
	if err := m.q.List.SelectContext(ctx, &out); err != nil {
		m.logError("error listing secrets", "error", err)
		return nil, err
	}
	return out, nil
}

// Create encrypts and stores a new secret value.
func (m *Manager) Create(ctx context.Context, name, plainValue string, createdBy int) error {
	name = strings.TrimSpace(name)
	if !ValidName(name) {
		return ErrInvalidName
	}
	if len([]byte(plainValue)) > maxValueBytes {
		return ErrValueTooLarge
	}
	ciphertext, nonce, err := Encrypt(m.kek, []byte(plainValue))
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	var createdByArg any
	if createdBy > 0 {
		createdByArg = null.IntFrom(createdBy)
	}
	var id int64
	if err := m.q.Insert.GetContext(ctx, &id, name, ciphertext, nonce, createdByArg); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return ErrSecretExists
		}
		m.logError("error creating secret", "name", name, "error", err)
		return err
	}
	return nil
}

// Update replaces an existing secret value.
func (m *Manager) Update(ctx context.Context, name, plainValue string) error {
	name = strings.TrimSpace(name)
	if !ValidName(name) {
		return ErrInvalidName
	}
	if len([]byte(plainValue)) > maxValueBytes {
		return ErrValueTooLarge
	}
	ciphertext, nonce, err := Encrypt(m.kek, []byte(plainValue))
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	res, err := m.q.Update.ExecContext(ctx, name, ciphertext, nonce)
	if err != nil {
		m.logError("error updating secret", "name", name, "error", err)
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// Delete removes a secret by name.
func (m *Manager) Delete(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if !ValidName(name) {
		return ErrInvalidName
	}
	res, err := m.q.Delete.ExecContext(ctx, name)
	if err != nil {
		m.logError("error deleting secret", "name", name, "error", err)
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// Get decrypts a single secret value for SDK use.
func (m *Manager) Get(ctx context.Context, name string, access AccessContext) (string, bool, error) {
	name = strings.TrimSpace(name)
	if !ValidName(name) {
		return "", false, ErrInvalidName
	}
	var row encryptedSecret
	if err := m.q.Get.GetContext(ctx, &row, name); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		m.logError("error fetching secret", "name", name, "error", err)
		return "", false, err
	}
	value, err := Decrypt(m.kek, row.EncryptedValue, row.Nonce)
	if err != nil {
		m.logError("secret decrypt failed", "name", name, "error", err)
		return "", false, nil
	}
	m.logAccess(name, access)
	if _, err := m.q.Touch.ExecContext(ctx, name); err != nil {
		m.logError("error updating secret access timestamp", "name", name, "error", err)
		return "", false, err
	}
	return string(value), true, nil
}

// LoadAll decrypts every configured secret for one sandbox execution.
// Corrupted rows are logged and omitted so one bad secret cannot break all
// executor runs.
func (m *Manager) LoadAll(ctx context.Context, access AccessContext) (map[string]string, error) {
	rows := make([]encryptedSecret, 0)
	if err := m.q.LoadAll.SelectContext(ctx, &rows); err != nil {
		m.logError("error loading secrets", "error", err)
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		value, err := Decrypt(m.kek, row.EncryptedValue, row.Nonce)
		if err != nil {
			m.logError("secret decrypt failed", "name", row.Name, "error", err)
			continue
		}
		out[row.Name] = string(value)
		m.logAccess(row.Name, access)
		if _, err := m.q.Touch.ExecContext(ctx, row.Name); err != nil {
			m.logError("error updating secret access timestamp", "name", row.Name, "error", err)
			return nil, err
		}
	}
	return out, nil
}

func (m *Manager) logAccess(name string, access AccessContext) {
	if m == nil || m.lo == nil {
		return
	}
	m.lo.Info("secret access",
		"secret_name", name,
		"function_name", access.FunctionName,
		"conversation_id", access.ConversationID,
		"trace_id", access.TraceID,
	)
}

func (m *Manager) logError(msg string, args ...any) {
	if m == nil || m.lo == nil {
		return
	}
	m.lo.Error(msg, args...)
}
