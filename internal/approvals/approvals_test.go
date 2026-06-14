package approvals

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/aziz-tb/merio/internal/approvals/models"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/logf"
)

func newTestManager(t *testing.T) (*Manager, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	dbx := sqlx.NewDb(db, "sqlmock")
	mock.MatchExpectationsInOrder(false)
	for i := 0; i < 4; i++ { // insert, get-by-id, list-pending-by-conversation, resolve
		mock.ExpectPrepare(".*")
	}
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	m, err := New(Opts{DB: dbx, Lo: &lo})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	mock.MatchExpectationsInOrder(true)
	return m, mock
}

func approvalRow(status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "conversation_id", "gated_run_trace_id", "parent_trace_id", "agent_id",
		"tool_id", "tool_use_id", "proposed_inputs", "status", "approved_inputs",
		"rejection_reason", "reviewed_by_user_id", "resolved_at", "created_at", "updated_at",
	}).AddRow(
		int64(1), int64(5), "trace-1", nil, nil,
		7, "w1", []byte(`{"id":1}`), status, []byte(`{"id":1}`),
		nil, int64(9), now, now, now,
	)
}

func i64ptr(v int64) *int64 { return &v }

// TestResolve_IdempotencyLock proves the atomic status transition is the lock:
// the first resolve flips pending → approved and returns the row; a second
// (double-click / duplicate) resolve matches zero pending rows and returns
// ErrNotPending, so the caller never starts a second resuming run and the write
// cannot execute twice.
func TestResolve_IdempotencyLock(t *testing.T) {
	m, mock := newTestManager(t)

	// 1st resolve — UPDATE ... WHERE status='pending' matches and RETURNS the row.
	mock.ExpectQuery("UPDATE agent_write_approvals").
		WillReturnRows(approvalRow(models.StatusApproved))
	first, err := m.Resolve(ResolveParams{
		ID: 1, Status: models.StatusApproved,
		ApprovedInputs: json.RawMessage(`{"id":1}`), ReviewedByUserID: i64ptr(9),
	})
	require.NoError(t, err)
	require.Equal(t, models.StatusApproved, first.Status)

	// 2nd resolve — the WHERE status='pending' no longer matches → zero rows.
	// The manager confirms the row still exists (so it's not a 404) and returns
	// ErrNotPending.
	mock.ExpectQuery("UPDATE agent_write_approvals").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT").
		WillReturnRows(approvalRow(models.StatusApproved))
	_, err = m.Resolve(ResolveParams{
		ID: 1, Status: models.StatusApproved,
		ApprovedInputs: json.RawMessage(`{"id":1}`), ReviewedByUserID: i64ptr(9),
	})
	require.ErrorIs(t, err, ErrNotPending)
	require.NoError(t, mock.ExpectationsWereMet())
}
