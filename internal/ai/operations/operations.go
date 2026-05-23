// Package aioperations is the persistence layer for ai_operations, the
// unified per-logical-AI-operation outcome audit (Batch 9). Feature
// consumers (tag classification today; agent validation, OCR, … later)
// build an Operation with an operation-specific payload and call Insert.
// Reads power per-conversation AI-activity views.
package aioperations

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abhinavxd/libredesk/internal/ai/operations/models"
	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// ErrNotFound is returned by GetByID when no row matches.
var ErrNotFound = errors.New("aioperations: row not found")

// Manager is the DB-backed CRUD façade for ai_operations.
type Manager struct {
	db *sqlx.DB
	q  queries
	lo *logf.Logger
}

// Opts wires dependencies.
type Opts struct {
	DB *sqlx.DB
	Lo *logf.Logger
}

type queries struct {
	Insert              *sqlx.Stmt `query:"insert"`
	GetByID             *sqlx.Stmt `query:"get-by-id"`
	ListForConversation *sqlx.Stmt `query:"list-for-conversation"`
	ListByTypeForConv   *sqlx.Stmt `query:"list-by-type-for-conversation"`
}

// New builds a Manager with prepared statements.
func New(opts Opts) (*Manager, error) {
	if opts.DB == nil {
		return nil, errors.New("aioperations: DB is required")
	}
	if opts.Lo == nil {
		return nil, errors.New("aioperations: Lo is required")
	}
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, fmt.Errorf("aioperations: prepare queries: %w", err)
	}
	return &Manager{db: opts.DB, q: q, lo: opts.Lo}, nil
}

// Insert writes one ai_operations row and returns its DB-generated id.
// Payload must be the marshaled JSON for the operation type; empty
// payload is normalized to '{}'.
func (m *Manager) Insert(ctx context.Context, op models.Operation) (int64, error) {
	payload := op.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var id int64
	if err := m.q.Insert.GetContext(ctx, &id,
		op.OperationType,
		op.ConversationID,
		op.TriggeredBy,
		op.TriggeredByUserID,
		op.TraceID,
		op.Status,
		op.ErrorMessage,
		[]byte(payload),
	); err != nil {
		m.lo.Error("error inserting ai_operation",
			"operation_type", op.OperationType, "error", err)
		return 0, err
	}
	return id, nil
}

// GetByID loads one operation. Returns ErrNotFound when missing.
func (m *Manager) GetByID(ctx context.Context, id int64) (models.Operation, error) {
	var op models.Operation
	if err := m.q.GetByID.GetContext(ctx, &op, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return op, ErrNotFound
		}
		m.lo.Error("error fetching ai_operation", "id", id, "error", err)
		return op, err
	}
	return op, nil
}

// ListForConversation returns operations for a conversation, newest first.
// limit defaults to 50 and is capped at 200.
func (m *Manager) ListForConversation(ctx context.Context, convID int64, limit int) ([]models.Operation, error) {
	limit = clampLimit(limit)
	out := make([]models.Operation, 0)
	if err := m.q.ListForConversation.SelectContext(ctx, &out, convID, limit); err != nil {
		m.lo.Error("error listing ai_operations for conversation", "conversation_id", convID, "error", err)
		return nil, err
	}
	return out, nil
}

// ListByTypeForConversation filters ListForConversation to one
// operation_type. Used to find the latest op of a kind on a conversation.
func (m *Manager) ListByTypeForConversation(ctx context.Context, convID int64, opType string, limit int) ([]models.Operation, error) {
	limit = clampLimit(limit)
	out := make([]models.Operation, 0)
	if err := m.q.ListByTypeForConv.SelectContext(ctx, &out, convID, opType, limit); err != nil {
		m.lo.Error("error listing ai_operations by type", "conversation_id", convID, "operation_type", opType, "error", err)
		return nil, err
	}
	return out, nil
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}
