package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_6 introduces the Composer tab's persistence layer — two tables
// that record (1) every agent run on a conversation and (2) what was
// done with each run (sent / drafted / escalated / discarded).
//
// Design rationale (kept here so future readers don't have to dig
// through the implementation prompt): one table doesn't fit L2/L3
// autonomy or pipeline-driven runs cleanly. An execution is the
// immutable artifact of a runner invocation; an outcome is the
// downstream commitment made on top of it. Separating them lets the
// outcome ship later (pipeline) or evolve independently (multiple
// outcome kinds) without rewriting the run record. The UNIQUE
// constraint on outcomes.agent_execution_id enforces at-most-one
// commitment per run.
//
//   - agent_executions: row inserted on every Run() success. Never
//     updated. Regenerate = new row. Carries agent_updated_at so the UI
//     can detect when an agent has been edited since the run was
//     generated (the "stale" indicator).
//   - agent_execution_outcomes: row inserted when an operator commits
//     to a run (clicks Send reply, today; pipeline-side outcomes when
//     L2/L3 ship). UNIQUE on agent_execution_id keeps the relationship
//     authoritative.
//
// Indexes are sized for two access patterns:
//   - (conversation_id, agent_id, run_at DESC) — the Composer tab's
//     "latest run per agent per conversation" load path.
//   - (source, run_at DESC) — analytics. Negligible cost today; useful
//     once pipeline rows accumulate.
//
// resolution_attempt_id is nullable INT with no FK yet — F3 (the
// inbound pipeline's resolution_attempt table) hasn't shipped. The
// column reserves the slot so the pipeline doesn't need another
// migration to wire its runs.
//
// Forward-only. Idempotent via IF NOT EXISTS. Rollback:
//
//	BEGIN;
//	DROP TABLE IF EXISTS agent_execution_outcomes;
//	DROP TABLE IF EXISTS agent_executions;
//	COMMIT;
func V2_7_6(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_executions (
			id                    BIGSERIAL PRIMARY KEY,
			conversation_id       BIGINT NOT NULL
			                      REFERENCES conversations(id) ON DELETE CASCADE,
			agent_id              INT NOT NULL
			                      REFERENCES ai_agents(id) ON DELETE RESTRICT,
			source                TEXT NOT NULL,
			resolution_attempt_id INT NULL,
			trace_id              TEXT NOT NULL DEFAULT '',
			status                TEXT NOT NULL,
			draft                 TEXT NOT NULL DEFAULT '',
			trace                 JSONB NOT NULL DEFAULT '[]'::jsonb,
			usage                 JSONB NOT NULL DEFAULT '{}'::jsonb,
			duration_ms           INT NOT NULL DEFAULT 0,
			run_by_user_id        BIGINT NULL
			                      REFERENCES users(id) ON DELETE SET NULL,
			run_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			agent_updated_at      TIMESTAMPTZ NOT NULL,
			created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT constraint_agent_executions_on_source
			    CHECK (source IN ('composer', 'pipeline'))
		);

		CREATE INDEX IF NOT EXISTS index_agent_executions_on_conv_agent_run_at
			ON agent_executions (conversation_id, agent_id, run_at DESC);
		CREATE INDEX IF NOT EXISTS index_agent_executions_on_source_run_at
			ON agent_executions (source, run_at DESC);
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_execution_outcomes (
			id                   BIGSERIAL PRIMARY KEY,
			agent_execution_id   BIGINT NOT NULL UNIQUE
			                     REFERENCES agent_executions(id) ON DELETE CASCADE,
			outcome              TEXT NOT NULL,
			outcome_reason       TEXT NULL,
			message_id           BIGINT NULL
			                     REFERENCES conversation_messages(id) ON DELETE SET NULL,
			committed_by_user_id BIGINT NULL
			                     REFERENCES users(id) ON DELETE SET NULL,
			committed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT constraint_agent_execution_outcomes_on_outcome
			    CHECK (outcome IN ('sent', 'drafted', 'escalated', 'discarded'))
		);

		CREATE INDEX IF NOT EXISTS index_agent_execution_outcomes_on_outcome_committed_at
			ON agent_execution_outcomes (outcome, committed_at DESC);
	`); err != nil {
		return err
	}

	return nil
}
