package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_12 creates the functions table — the single, unified home for every
// named, sandbox-runnable code blob across the platform (Unified Executor
// Framework batch). Previously code lived fragmented across
// ai_tools.implementation (custom AI tools) and customer_identification
// .executor.* settings rows. After this batch every code blob is one
// functions row addressed by a globally-unique name.
//
//   - domain is a controlled enum (ai_tool, customer_identification);
//     future domains (automation, …) add their value by their own batch.
//   - language is enumerated to what internal/exec/runner accepts today
//     (python, javascript). The runner takes these values directly — there
//     is no mapping layer.
//   - parameters is the {id, key, value} array used by preview/test runs.
//
// Idempotent via CREATE TABLE / INDEX IF NOT EXISTS. Forward-only.
// Rollback: DROP TABLE IF EXISTS functions;
func V2_8_12(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS functions (
		    id              BIGSERIAL PRIMARY KEY,
		    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

		    name            TEXT NOT NULL,
		    domain          TEXT NOT NULL,

		    code            TEXT NOT NULL DEFAULT '',
		    language        TEXT NOT NULL DEFAULT 'python',
		    packages        TEXT NOT NULL DEFAULT '',
		    timeout_ms      INT  NOT NULL DEFAULT 30000,
		    enabled         BOOLEAN NOT NULL DEFAULT TRUE,

		    parameters      JSONB NOT NULL DEFAULT '[]'::jsonb,

		    updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,

		    CONSTRAINT constraint_functions_on_name_unique
		        UNIQUE (name),
		    CONSTRAINT check_functions_language
		        CHECK (language IN ('python', 'javascript')),
		    CONSTRAINT check_functions_timeout
		        CHECK (timeout_ms > 0 AND timeout_ms <= 120000),
		    CONSTRAINT check_functions_domain
		        CHECK (domain IN ('ai_tool', 'customer_identification'))
		);
		CREATE INDEX IF NOT EXISTS index_functions_on_domain ON functions (domain);
	`); err != nil {
		return err
	}
	return nil
}
