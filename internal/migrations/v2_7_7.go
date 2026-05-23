package migrations

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// systemInboxUUID is the stable identifier for the seeded "System
// (Internal)" inbox introduced by this migration. ai_usage rows that
// have no real inbox context (rewrite manager calls, synthetic test
// runs from AgentTestPanel, future internal jobs) FK to this row. The
// UUID is hard-coded so the application can resolve it once at startup
// without depending on insertion order — and so the migration is
// idempotent across re-runs via ON CONFLICT (uuid) DO NOTHING.
const SystemInboxUUID = "00000000-0000-0000-0000-000000000001"

// V2_7_7 introduces the ai_usage instrumentation layer — one row per
// LLM provider call. Powers cost analysis, latency analysis, cache
// effectiveness, error analysis, per-inbox / per-agent rollups, and
// every future observability surface (otel exports, internal
// dashboards). Schema designed to handle 5–10M rows/year without
// retrofit.
//
// Three components ship together:
//
//   1. A seeded "System (Internal)" inbox — every ai_usage row needs a
//      valid inbox_id FK; non-conversation calls (rewrite manager,
//      AgentTestPanel synthetic runs) point at this sentinel row.
//      Channels enum has no "system" value so we use 'email' with
//      enabled=false to keep it out of inbox pickers.
//
//   2. The ai_usage table itself, declared as PARTITION BY RANGE
//      (created_at). Monthly partitions; pre-created for the current
//      month + 3 months ahead. The native Go partition manager
//      (internal/ai/usage/partitions) keeps the runway extended at
//      app startup and on a daily timer. pg_partman would be cleaner
//      but the postgres:17-alpine image doesn't bundle it — fallback
//      documented in docs/ai-usage.md.
//
//   3. All indexes declared on the parent table — PG14+ propagates
//      them to every current and future partition automatically.
//
// Idempotency constraint note: the natural unique index is
// (trace_id, call_sequence_in_trace) but partitioned tables in PG
// require unique indexes to include the partition key. Adopting
// (trace_id, call_sequence_in_trace, created_at). The application
// captures created_at ONCE per LLM call so cross-partition collisions
// are impossible; insert retries use the same stamped value.
//
// Forward-only. Idempotent via IF NOT EXISTS / ON CONFLICT.
// Rollback (manual):
//
//	BEGIN;
//	DROP TABLE IF EXISTS ai_usage CASCADE;  -- drops every partition
//	-- the System inbox stays; harmless and may be reused.
//	COMMIT;
func V2_7_7(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Seed the System inbox. Must exist before the FK constraint on
	//    ai_usage.inbox_id is enforced. ON CONFLICT (uuid) DO NOTHING
	//    keeps re-runs safe.
	if _, err := db.Exec(`
		INSERT INTO inboxes (uuid, name, channel, enabled, config, "from")
		VALUES ($1, 'System (Internal)', 'email', FALSE, '{}'::jsonb, '')
		ON CONFLICT (uuid) DO NOTHING;
	`, SystemInboxUUID); err != nil {
		return fmt.Errorf("v2_7_7: seed system inbox: %w", err)
	}

	// 2. Create the partitioned parent table. CREATE TABLE IF NOT
	//    EXISTS isn't supported for partitioned tables in some PG
	//    versions when combined with PARTITION BY — guard explicitly.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_usage (
			id                      BIGSERIAL NOT NULL,
			schema_version          SMALLINT NOT NULL DEFAULT 1,
			created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			trace_id                TEXT NOT NULL,
			call_sequence_in_trace  INT NOT NULL,

			inbox_id                INT NOT NULL,
			conversation_id         BIGINT NULL,
			agent_id                INT NULL,
			agent_execution_id      BIGINT NULL,
			resolution_attempt_id   INT NULL,
			user_id                 BIGINT NULL,

			provider                TEXT NOT NULL,
			model                   TEXT NOT NULL,
			preset_slug             TEXT NOT NULL,

			purpose                 TEXT NOT NULL,
			stage                   TEXT NULL,
			stop_reason             TEXT NOT NULL DEFAULT '',

			input_tokens            INT NOT NULL DEFAULT 0,
			output_tokens           INT NOT NULL DEFAULT 0,
			cached_input_tokens     INT NOT NULL DEFAULT 0,
			cache_markers_used      INT NOT NULL DEFAULT 0,

			input_cost_usd          NUMERIC(12, 8) NOT NULL DEFAULT 0,
			output_cost_usd         NUMERIC(12, 8) NOT NULL DEFAULT 0,
			cached_input_cost_usd   NUMERIC(12, 8) NOT NULL DEFAULT 0,
			total_cost_usd          NUMERIC(12, 8) NOT NULL DEFAULT 0,

			latency_ms              INT NOT NULL DEFAULT 0,
			provider_latency_ms     INT NULL,
			retry_count             INT NOT NULL DEFAULT 0,

			outcome                 TEXT NOT NULL,
			error_class             TEXT NULL,
			error_message           TEXT NULL,

			provider_request_id     TEXT NULL,
			provider_metadata       JSONB NULL,

			request_payload         JSONB NULL,
			response_payload        JSONB NULL,
			structured_output       JSONB NULL,

			-- Partitioned tables can't carry a PK that doesn't include the
			-- partition key. The natural identity is (id), but since the
			-- application never queries by id alone, we leave the table
			-- without a declared PK; id remains a sequence-backed unique
			-- value scoped per-partition.
			CONSTRAINT constraint_ai_usage_on_call_sequence CHECK (call_sequence_in_trace > 0),
			CONSTRAINT constraint_ai_usage_on_input_tokens  CHECK (input_tokens >= 0),
			CONSTRAINT constraint_ai_usage_on_output_tokens CHECK (output_tokens >= 0),
			CONSTRAINT constraint_ai_usage_on_cached_tokens CHECK (cached_input_tokens >= 0),
			CONSTRAINT constraint_ai_usage_on_latency_ms    CHECK (latency_ms >= 0),
			CONSTRAINT constraint_ai_usage_on_retry_count   CHECK (retry_count >= 0),
			CONSTRAINT constraint_ai_usage_on_total_cost    CHECK (total_cost_usd >= 0),
			CONSTRAINT constraint_ai_usage_on_outcome CHECK (
			    outcome IN ('success', 'timeout', 'auth_error', 'rate_limited',
			                'circuit_open', 'invalid_request', 'other')
			)
		) PARTITION BY RANGE (created_at);
	`); err != nil {
		return fmt.Errorf("v2_7_7: create ai_usage: %w", err)
	}

	// 3. FK constraints. ON DELETE RESTRICT for inbox (every row must
	//    keep its inbox provenance even if a real inbox is later
	//    deleted — operators rarely do this anyway); SET NULL elsewhere
	//    so cost records survive agent / conversation / user deletes.
	for _, alter := range []string{
		`ALTER TABLE ai_usage
		    ADD CONSTRAINT fk_ai_usage_on_inbox
		    FOREIGN KEY (inbox_id) REFERENCES inboxes(id) ON DELETE RESTRICT;`,
		`ALTER TABLE ai_usage
		    ADD CONSTRAINT fk_ai_usage_on_conversation
		    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL;`,
		`ALTER TABLE ai_usage
		    ADD CONSTRAINT fk_ai_usage_on_agent
		    FOREIGN KEY (agent_id) REFERENCES ai_agents(id) ON DELETE SET NULL;`,
		`ALTER TABLE ai_usage
		    ADD CONSTRAINT fk_ai_usage_on_agent_execution
		    FOREIGN KEY (agent_execution_id) REFERENCES agent_executions(id) ON DELETE SET NULL;`,
		`ALTER TABLE ai_usage
		    ADD CONSTRAINT fk_ai_usage_on_user
		    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;`,
	} {
		if _, err := db.Exec(alter); err != nil {
			// IF NOT EXISTS isn't valid for ADD CONSTRAINT; swallow the
			// duplicate-object error so re-runs stay idempotent.
			if !isDuplicateObject(err) {
				return fmt.Errorf("v2_7_7: add FK: %w", err)
			}
		}
	}

	// 4. Indexes. Declared on the parent; PG14+ propagates to children.
	for _, idx := range []string{
		// Idempotency. Partitioned tables require the partition key in
		// any unique index — created_at is the natural ordering field
		// for retries anyway (caller stamps once per LLM call).
		`CREATE UNIQUE INDEX IF NOT EXISTS index_ai_usage_on_trace_seq
		    ON ai_usage (trace_id, call_sequence_in_trace, created_at);`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_created_at
		    ON ai_usage (created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_inbox_created
		    ON ai_usage (inbox_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_agent_created
		    ON ai_usage (agent_id, created_at DESC) WHERE agent_id IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_agent_execution
		    ON ai_usage (agent_execution_id) WHERE agent_execution_id IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_trace
		    ON ai_usage (trace_id);`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_purpose_model_created
		    ON ai_usage (purpose, model, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_errors
		    ON ai_usage (outcome, created_at DESC) WHERE outcome != 'success';`,
		`CREATE INDEX IF NOT EXISTS index_ai_usage_on_provider_model
		    ON ai_usage (provider, model, created_at DESC);`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("v2_7_7: create index: %w", err)
		}
	}

	// 5. Pre-create the current month + 3 future months of partitions.
	//    The application-side partition manager
	//    (internal/ai/usage/partitions) keeps the runway extended past
	//    boot via a daily check.
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		start := time.Date(now.Year(), now.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		partition := fmt.Sprintf("ai_usage_%04d_%02d", start.Year(), start.Month())
		create := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s PARTITION OF ai_usage
			    FOR VALUES FROM ('%s') TO ('%s');
		`, partition, start.Format("2006-01-02"), end.Format("2006-01-02"))
		if _, err := db.Exec(create); err != nil {
			return fmt.Errorf("v2_7_7: create partition %s: %w", partition, err)
		}
	}

	return nil
}

// isDuplicateObject reports whether err is a Postgres
// duplicate_object (42710) — used to make ADD CONSTRAINT idempotent
// when re-running the migration.
func isDuplicateObject(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 42710") || strings.Contains(msg, "already exists")
}
