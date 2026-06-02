// Package usage is the write surface for the ai_usage table — one row
// per LLM provider call. The runner (and any future call site) builds
// a Row and calls Manager.Record(). Cost calculation happens inside
// Record(): the model id is looked up in the catalog, per-million-
// token rates are applied to the captured token counts, and the four
// cost fields land on the row before insert.
//
// Two non-obvious design choices:
//
//  1. Synchronous write. ~5ms on a healthy DB. We treat ai_usage
//     writes as load-bearing for audit + cost analysis and would
//     rather block the run for 5ms than risk losing rows in a queue
//     under back-pressure. If volume outgrows this assumption, swap
//     to a buffered async writer; the Manager interface stays stable.
//
//  2. Idempotency leans on the partial unique index
//     (trace_id, call_sequence_in_trace, created_at). The caller
//     stamps created_at ONCE per LLM call so a retry of the insert
//     uses the same value and trips the unique constraint cleanly.
//     Cross-partition collisions are not possible because no caller
//     generates two rows with the same (trace_id, seq) and different
//     created_at.
package usage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm/catalog"
	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// Outcome values. Mirrors the CHECK constraint on ai_usage.outcome.
const (
	OutcomeSuccess        = "success"
	OutcomeTimeout        = "timeout"
	OutcomeAuthError      = "auth_error"
	OutcomeRateLimited    = "rate_limited"
	OutcomeCircuitOpen    = "circuit_open"
	OutcomeInvalidRequest = "invalid_request"
	OutcomeOther          = "other"
)

// ErrDuplicate signals an idempotent retry: the row was already
// recorded for (trace_id, call_sequence_in_trace, created_at). The
// Manager swallows this and logs at INFO; callers don't need to
// branch on it but the type is exposed for tests.
var ErrDuplicate = errors.New("usage: duplicate (trace_id, call_sequence, created_at)")

// Manager is the canonical write surface for ai_usage.
type Manager struct {
	db      *sqlx.DB
	catalog *catalog.Catalog
	lo      *logf.Logger
	q       queries
}

// Opts wires dependencies.
type Opts struct {
	DB      *sqlx.DB
	Catalog *catalog.Catalog
	Lo      *logf.Logger
}

type queries struct {
	Insert *sqlx.Stmt `query:"insert-usage"`
}

// New builds a Manager with the insert statement prepared.
func New(opt Opts) (*Manager, error) {
	if opt.DB == nil {
		return nil, errors.New("usage: DB is required")
	}
	if opt.Catalog == nil {
		return nil, errors.New("usage: Catalog is required (cost calc depends on it)")
	}
	if opt.Lo == nil {
		return nil, errors.New("usage: Lo is required")
	}
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, fmt.Errorf("usage: prepare insert: %w", err)
	}
	return &Manager{db: opt.DB, catalog: opt.Catalog, lo: opt.Lo, q: q}, nil
}

// Row is the input to Manager.Record. The runner / handler populates
// every field they have access to. Cost columns are computed by Record
// — the caller doesn't need to pre-populate them.
//
// CreatedAt drives the partition target. Callers MUST set it once at
// build time (right after the LLM call returns) and reuse the same
// value on retries. Leaving it zero means "use time.Now()" — fine for
// the runner's normal path but not for idempotency-conscious paths.
type Row struct {
	CreatedAt time.Time

	TraceID             string
	CallSequenceInTrace int

	InboxID             int
	ConversationID      *int64
	AgentID             *int
	AgentExecutionID    *int64
	ResolutionAttemptID *int
	UserID              *int64

	Provider   string
	Model      string
	PresetSlug string

	Purpose    string
	StopReason string

	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	CacheMarkersUsed  int

	LatencyMs         int
	ProviderLatencyMs *int
	RetryCount        int

	Outcome      string
	ErrorClass   *string
	ErrorMessage *string

	ProviderRequestID *string
	ProviderMetadata  json.RawMessage

	RequestPayload   json.RawMessage
	ResponsePayload  json.RawMessage
	StructuredOutput json.RawMessage
}

// Record writes one ai_usage row. Synchronous. Idempotent — a retry
// with the same (trace_id, call_sequence, created_at) returns nil with
// ErrDuplicate logged at INFO.
//
// Cost calculation:
//   - Resolves the model in the catalog
//   - input_cost  = (input_tokens / 1e6)         * input_per_million_usd
//   - output_cost = (output_tokens / 1e6)        * output_per_million_usd
//   - cached_cost = (cached_input_tokens / 1e6)  * cached_input_per_million_usd
//   - total       = input + output + cached
//
// If the model isn't in the catalog (shouldn't happen given the
// startup validator) costs are all zero and a WARN is logged. The row
// still lands — losing cost detail is preferable to dropping audit.
func (m *Manager) Record(ctx context.Context, row Row) error {
	if row.TraceID == "" {
		return errors.New("usage: trace_id is required")
	}
	if row.CallSequenceInTrace <= 0 {
		return errors.New("usage: call_sequence_in_trace must be > 0")
	}
	if row.InboxID == 0 {
		return errors.New("usage: inbox_id is required (use SystemInboxUUID's id for non-conversation calls)")
	}
	if row.Provider == "" || row.Model == "" || row.PresetSlug == "" {
		return errors.New("usage: provider, model, preset_slug all required")
	}
	if row.Purpose == "" {
		return errors.New("usage: purpose is required")
	}
	if row.Outcome == "" {
		row.Outcome = OutcomeOther
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}

	// Cost calc. Catalog lookup is in-memory; cheap.
	inputCost, outputCost, cachedCost, ok := ComputeCost(m.catalog, row.Model,
		row.InputTokens, row.OutputTokens, row.CachedInputTokens)
	if !ok {
		m.lo.Warn("usage: model not in catalog; cost columns will be zero",
			"model", row.Model, "trace_id", row.TraceID)
	}
	totalCost := inputCost + outputCost + cachedCost

	// Normalize JSONB nullables — Postgres distinguishes NULL from
	// 'null'::jsonb, and our schema doesn't NOT NULL them, so unset
	// fields should land as NULL (not the literal string 'null').
	var providerMetadata interface{}
	if len(row.ProviderMetadata) > 0 {
		providerMetadata = []byte(row.ProviderMetadata)
	}
	var requestPayload interface{}
	if len(row.RequestPayload) > 0 {
		requestPayload = []byte(row.RequestPayload)
	}
	var responsePayload interface{}
	if len(row.ResponsePayload) > 0 {
		responsePayload = []byte(row.ResponsePayload)
	}
	var structuredOutput interface{}
	if len(row.StructuredOutput) > 0 {
		structuredOutput = []byte(row.StructuredOutput)
	}

	_, err := m.q.Insert.ExecContext(ctx,
		row.CreatedAt,
		row.TraceID,
		row.CallSequenceInTrace,
		row.InboxID,
		nilOrInt64(row.ConversationID),
		nilOrInt(row.AgentID),
		nilOrInt64(row.AgentExecutionID),
		nilOrInt(row.ResolutionAttemptID),
		nilOrInt64(row.UserID),
		row.Provider,
		row.Model,
		row.PresetSlug,
		row.Purpose,
		row.StopReason,
		row.InputTokens,
		row.OutputTokens,
		row.CachedInputTokens,
		row.CacheMarkersUsed,
		inputCost,
		outputCost,
		cachedCost,
		totalCost,
		row.LatencyMs,
		nilOrInt(row.ProviderLatencyMs),
		row.RetryCount,
		row.Outcome,
		nilOrString(row.ErrorClass),
		nilOrString(row.ErrorMessage),
		nilOrString(row.ProviderRequestID),
		providerMetadata,
		requestPayload,
		responsePayload,
		structuredOutput,
	)
	if err != nil {
		// Postgres unique violation (23505) on the partial unique
		// index → idempotent retry of the same logical write.
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			m.lo.Info("usage: duplicate row swallowed (idempotency)",
				"trace_id", row.TraceID, "seq", row.CallSequenceInTrace)
			return nil
		}
		// Don't return the raw error to the caller — wrapping keeps the
		// runner's error path predictable.
		return fmt.Errorf("usage: insert: %w", err)
	}
	return nil
}

// ComputeCost is the pure-math cost calculator extracted so tests can
// exercise it without a DB. Looks up the model in the catalog, applies
// per-million-token rates to the captured counts, returns (input,
// output, cached) costs in USD plus a found flag. When the model
// isn't in the catalog every cost is zero and ok=false; callers
// decide whether to log or fail.
//
// Convention: rates are per-million tokens. (tokens / 1e6) * rate.
func ComputeCost(cat *catalog.Catalog, modelID string, inputTokens, outputTokens, cachedInputTokens int) (input, output, cached float64, ok bool) {
	if cat == nil {
		return 0, 0, 0, false
	}
	m, found := cat.Get(modelID)
	if !found {
		return 0, 0, 0, false
	}
	const million = 1_000_000.0
	input = (float64(inputTokens) / million) * m.Pricing.InputPerMillionTokensUSD
	output = (float64(outputTokens) / million) * m.Pricing.OutputPerMillionTokensUSD
	cached = (float64(cachedInputTokens) / million) * m.Pricing.CachedInputPerMillionTokensUSD
	return input, output, cached, true
}

// Helper functions to convert nullable Go pointers into interface{}
// values that sqlx will pass as SQL NULL rather than the zero value.

func nilOrInt(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func nilOrInt64(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func nilOrString(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// Guard against unused-import warning when no rows query lands here —
// keep sql import wired for future Get / Count methods.
var _ = sql.ErrNoRows
