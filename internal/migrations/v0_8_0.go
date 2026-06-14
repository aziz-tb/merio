package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_8_0 adds agent_write_approvals — the durable record of a proposed write
// tool call awaiting human approval (Slice 4, run-ends-at-write). References use
// FK ids (agent_id, tool_id); the gate and the LLM tool call stay slug-driven.
// gated_run_trace_id is the run this approval gates; parent_trace_id is the
// explicit composer run a sub-agent gate belongs to (so resume never parses the
// ":sub:" trace-id encoding). The partial-unique index allows at most one open
// approval per gated run. Forward-only and idempotent.
func V0_8_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_write_approvals (
			id                   BIGSERIAL PRIMARY KEY,
			conversation_id      BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			gated_run_trace_id   TEXT NOT NULL,
			parent_trace_id      TEXT NULL,
			agent_id             INT NULL REFERENCES ai_agents(id) ON DELETE SET NULL,
			tool_id              INT NOT NULL REFERENCES ai_tools(id) ON DELETE CASCADE,
			tool_use_id          TEXT NOT NULL,
			proposed_inputs      JSONB NOT NULL DEFAULT '{}'::jsonb,
			status               TEXT NOT NULL DEFAULT 'pending',
			approved_inputs      JSONB NULL,
			rejection_reason     TEXT NULL,
			reviewed_by_user_id  BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
			resolved_at          TIMESTAMPTZ NULL,
			created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT constraint_agent_write_approvals_on_status
			    CHECK (status IN ('pending', 'approved', 'edited', 'rejected'))
		);

		CREATE INDEX IF NOT EXISTS index_agent_write_approvals_on_conversation
			ON agent_write_approvals (conversation_id, status);

		CREATE UNIQUE INDEX IF NOT EXISTS index_agent_write_approvals_one_pending_per_run
			ON agent_write_approvals (gated_run_trace_id) WHERE status = 'pending';
	`)
	return err
}
