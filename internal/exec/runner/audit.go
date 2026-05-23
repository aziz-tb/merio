package runner

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/abhinavxd/libredesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
)

//go:embed queries.sql
var queriesFS embed.FS

// queries holds prepared statements scanned from queries.sql at boot.
type queries struct {
	InsertExecutionStmt *sqlx.Stmt `query:"insert-execution"`
}

// executionRow is the wire shape inserted into tool_executions (was
// script_executions before the F5 schema cleanup; column shape unchanged,
// the table holds audit rows for both Python custom tools and — once
// chunk 10's dispatcher unification lands — internal capabilities). The
// column order matches queries.sql exactly.
type executionRow struct {
	ExecutionID    string
	ToolName       *string
	InvokerUserID  int
	ConversationID *int
	ToolDepth      int
	Language       string
	StartedAt      time.Time
	FinishedAt     time.Time
	DurationMs    int
	Kind           string
	ResultCode     *string
	Message        *string
	Data           json.RawMessage // nullable JSONB
	WarmPoolHit    bool
	TraceID        *string
}

// initQueries reads queries.sql via dbutil and prepares the statements.
//
// Mirrors the manager pattern used by macro, conversation, etc. — the
// same `dbutil.ScanSQLFile` reflection-based binder.
func initQueries(db *sqlx.DB) (*queries, error) {
	q := &queries{}
	if err := dbutil.ScanSQLFile("queries.sql", q, db, queriesFS); err != nil {
		return nil, fmt.Errorf("scan queries.sql: %w", err)
	}
	return q, nil
}

// insertExecution writes one audit row.
//
// The Data column is JSONB and accepts NULL when the row represents a
// dispatch error (no script result to record). We pass nil rather than
// the empty JSON object so the column ends up as SQL NULL.
func (q *queries) insertExecution(ctx context.Context, row executionRow) error {
	var dataArg interface{} = row.Data
	if len(row.Data) == 0 {
		dataArg = nil
	}

	_, err := q.InsertExecutionStmt.ExecContext(ctx,
		row.ExecutionID,
		row.ToolName,
		row.InvokerUserID,
		row.ConversationID,
		row.ToolDepth,
		row.Language,
		row.StartedAt,
		row.FinishedAt,
		row.DurationMs,
		row.Kind,
		row.ResultCode,
		row.Message,
		dataArg,
		row.WarmPoolHit,
		row.TraceID,
	)
	return err
}
