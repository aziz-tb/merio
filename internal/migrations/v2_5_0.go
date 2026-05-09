package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_5_0 provisions the script-execution audit table (script_executions),
// the System Agent user, and grants the tools:call permission to the
// Admin role on upgraded instances. Idempotent.
//
// The libredesk migration framework is forward-only. To roll this back,
// run the SQL below manually in psql against the same database, then
// redeploy a build that no longer contains the exec/runner code:
//
//	BEGIN;
//	UPDATE roles
//	SET    permissions = ARRAY(
//	           SELECT UNNEST(permissions)
//	           EXCEPT SELECT UNNEST(ARRAY['tools:call'])
//	       );
//	DELETE FROM user_roles
//	WHERE  user_id IN (SELECT id FROM users WHERE email = 'system@libredesk.local');
//	DELETE FROM users
//	WHERE  email = 'system@libredesk.local' AND type = 'agent';
//	DROP INDEX IF EXISTS idx_script_executions_invoker_user_id;
//	DROP INDEX IF EXISTS idx_script_executions_started_at;
//	DROP INDEX IF EXISTS idx_script_executions_conversation_id;
//	DROP INDEX IF EXISTS idx_script_executions_tool_name;
//	DROP TABLE IF EXISTS script_executions;
//	COMMIT;
//
// Note: custom roles inheriting from Agent are NOT touched by the
// upgrade-time grant — only the seeded Admin role gets `tools:call`.
//
// SAFETY NOTE: The System Agent user reuses the Admin role's full
// permissions. If Admin permissions expand later, the System Agent
// silently gains those capabilities. Reviewers of future Admin
// permission changes should consider whether scripts should gain the
// new capability — and if not, this migration should be revisited to
// introduce a narrower System role.
func V2_5_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Audit table + indexes. IF NOT EXISTS guards make this safe on re-run.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS script_executions (
			execution_id     TEXT PRIMARY KEY,
			tool_name        TEXT,
			invoker_user_id  BIGINT NOT NULL
			                 REFERENCES users(id) ON DELETE RESTRICT,
			conversation_id  BIGINT
			                 REFERENCES conversations(id) ON DELETE SET NULL,
			tool_depth       INT NOT NULL DEFAULT 0,
			language         TEXT NOT NULL,
			started_at       TIMESTAMPTZ NOT NULL,
			finished_at      TIMESTAMPTZ,
			duration_ms      INT NOT NULL DEFAULT 0,
			kind             TEXT NOT NULL CHECK (kind IN ('success','failure','error','noop')),
			result_code      TEXT,
			message          TEXT,
			data             JSONB,
			warm_pool_hit    BOOLEAN NOT NULL DEFAULT FALSE,
			trace_id         TEXT,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_script_executions_tool_name
			ON script_executions(tool_name) WHERE tool_name IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_script_executions_conversation_id
			ON script_executions(conversation_id) WHERE conversation_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_script_executions_started_at
			ON script_executions(started_at DESC);
		CREATE INDEX IF NOT EXISTS idx_script_executions_invoker_user_id
			ON script_executions(invoker_user_id);
	`); err != nil {
		return err
	}

	// 2. System Agent user. Identified by a stable, reserved email — never
	//    rename it without updating initSystemUserID in cmd/init.go.
	//
	//    `password` column is NULL-able and unset (no real password ever
	//    matches NULL via bcrypt). Email uniqueness is a *partial* unique
	//    index (type='agent' AND deleted_at IS NULL), so WHERE NOT EXISTS
	//    is safer than ON CONFLICT here.
	if _, err := db.Exec(`
		INSERT INTO users (type, enabled, email, first_name, last_name, password)
		SELECT 'agent', TRUE, 'system@libredesk.local', 'System Agent', '', NULL
		WHERE NOT EXISTS (
			SELECT 1 FROM users
			WHERE email = 'system@libredesk.local'
			  AND type = 'agent' AND deleted_at IS NULL
		);
	`); err != nil {
		return err
	}

	// 3. Grant the Admin role to the System Agent (user_roles many-to-many).
	if _, err := db.Exec(`
		INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id
		FROM   users u, roles r
		WHERE  u.email = 'system@libredesk.local'
		  AND  u.type = 'agent' AND u.deleted_at IS NULL
		  AND  r.name = 'Admin'
		  AND  NOT EXISTS (
			SELECT 1 FROM user_roles ur
			WHERE ur.user_id = u.id AND ur.role_id = r.id
		);
	`); err != nil {
		return err
	}

	// 4. Append tools:call to Admin's permissions array. Idempotent guard
	//    matches the v2_3_0/v2_4_0 style.
	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY['tools:call']::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('tools:call' = ANY(permissions));
	`); err != nil {
		return err
	}

	return nil
}
