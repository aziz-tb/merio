package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_3_0 provisions the worklog instrumentation table (B1) and grants its
// permissions to the existing seeded roles on upgraded instances. Idempotent.
//
// The libredesk migration framework is forward-only. To roll this back,
// run the SQL below manually in psql against the same database, then
// redeploy a build that no longer contains the worklog code:
//
//	BEGIN;
//	UPDATE roles
//	SET    permissions = ARRAY(
//	           SELECT UNNEST(permissions)
//	           EXCEPT SELECT UNNEST(ARRAY[
//	               'worklog:view_own',
//	               'worklog:view_team',
//	               'worklog:view_all'
//	           ])
//	       );
//	DROP INDEX IF EXISTS index_worklogs_on_action_type;
//	DROP INDEX IF EXISTS index_worklogs_on_started_at;
//	DROP INDEX IF EXISTS index_worklogs_on_user_id;
//	DROP INDEX IF EXISTS index_worklogs_on_conversation_id;
//	DROP TABLE IF EXISTS worklogs;
//	COMMIT;
func V2_3_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Create the worklogs table. IF NOT EXISTS guards make this safe on re-run.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS worklogs (
			id                BIGSERIAL PRIMARY KEY,
			conversation_id   BIGINT NOT NULL
			                  REFERENCES conversations(id)
			                  ON DELETE CASCADE ON UPDATE CASCADE,
			user_id           BIGINT NULL
			                  REFERENCES users(id)
			                  ON DELETE SET NULL ON UPDATE CASCADE,
			started_at        TIMESTAMPTZ NOT NULL,
			ended_at          TIMESTAMPTZ NULL,
			duration_seconds  INT NULL,
			action_type       VARCHAR(32) NOT NULL,
			automation_level  VARCHAR(2) NOT NULL DEFAULT 'L0',
			context           JSONB NULL,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT constraint_worklogs_on_action_type
				CHECK (action_type IN ('write', 'status', 'note', 'forward')),
			CONSTRAINT constraint_worklogs_on_automation_level
				CHECK (automation_level IN ('L0','L1','L2','L3','L4','L5')),
			CONSTRAINT constraint_worklogs_on_duration_non_negative
				CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
			CONSTRAINT constraint_worklogs_on_ended_at_after_started_at
				CHECK (ended_at IS NULL OR ended_at >= started_at)
		);

		CREATE INDEX IF NOT EXISTS index_worklogs_on_conversation_id
			ON worklogs (conversation_id);
		CREATE INDEX IF NOT EXISTS index_worklogs_on_user_id
			ON worklogs (user_id);
		CREATE INDEX IF NOT EXISTS index_worklogs_on_started_at
			ON worklogs (started_at);
		CREATE INDEX IF NOT EXISTS index_worklogs_on_action_type
			ON worklogs (action_type);
	`); err != nil {
		return err
	}

	// 2. Grant the new permissions on existing seeded roles. The schema.sql
	//    seed handles fresh installs; this branch handles upgraded instances.
	//    The NOT-contains guards keep this strictly idempotent.
	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY['worklog:view_own']::TEXT[]
		WHERE  name = 'Agent'
		  AND  NOT ('worklog:view_own' = ANY(permissions));
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY[
		           'worklog:view_own',
		           'worklog:view_team',
		           'worklog:view_all'
		       ]::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('worklog:view_all' = ANY(permissions));
	`); err != nil {
		return err
	}

	return nil
}
