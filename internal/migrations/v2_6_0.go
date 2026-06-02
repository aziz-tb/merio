package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_6_0 provisions the AI configuration tables (ai_agents, ai_tools)
// for the F9 configuration data model, and grants the corresponding
// permissions on the seeded Admin role. Idempotent.
//
// The merio migration framework is forward-only. To roll this back,
// run the SQL below manually in psql against the same database, then
// redeploy a build that no longer references these tables:
//
//	BEGIN;
//	UPDATE roles
//	SET    permissions = ARRAY(
//	           SELECT UNNEST(permissions)
//	           EXCEPT SELECT UNNEST(ARRAY['ai_agents:manage','ai_tools:manage','ai_tools:invoke'])
//	       );
//	DROP INDEX IF EXISTS index_ai_tools_on_type;
//	DROP INDEX IF EXISTS index_ai_tools_on_enabled;
//	DROP INDEX IF EXISTS index_ai_agents_on_enabled;
//	DROP TABLE IF EXISTS ai_tools;
//	DROP TABLE IF EXISTS ai_agents;
//	COMMIT;
//
// Note: custom roles inheriting from Agent are NOT touched by the
// upgrade-time grant — only the seeded Admin role gets these
// permissions, matching the v2_5_0 precedent for tools:call.
func V2_6_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. ai_agents — narrative agent definitions (prompt + tools + KB scope +
	//    detection conditions). Detection JSONB reuses the automation_rules
	//    Rule/RuleGroup/RuleDetail shape so one condition language is
	//    maintained across the platform. kb_folder_ids references kb_folders.id
	//    (BIGSERIAL) — validated at the application layer (Postgres can't FK
	//    into array elements).
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_agents (
			id                   SERIAL PRIMARY KEY,
			created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			name                 TEXT NOT NULL,
			slug                 TEXT NOT NULL,
			description          TEXT NULL,
			prompt               TEXT NOT NULL DEFAULT '',
			detection            JSONB NOT NULL DEFAULT '{}'::jsonb,
			tools                JSONB NOT NULL DEFAULT '[]'::jsonb,
			kb_folder_ids        BIGINT[] NOT NULL DEFAULT '{}',
			model_preset         TEXT NOT NULL DEFAULT 'default',
			confidence_threshold NUMERIC(4,3) NOT NULL DEFAULT 0.5,
			max_iterations       INT NOT NULL DEFAULT 5,
			autonomy             JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled              BOOL NOT NULL DEFAULT TRUE,
			CONSTRAINT constraint_ai_agents_on_name        CHECK (length(name) <= 140),
			CONSTRAINT constraint_ai_agents_on_slug        CHECK (length(slug) <= 140),
			CONSTRAINT constraint_ai_agents_on_description CHECK (description IS NULL OR length(description) <= 1000),
			CONSTRAINT constraint_ai_agents_on_threshold   CHECK (confidence_threshold >= 0 AND confidence_threshold <= 1),
			CONSTRAINT constraint_ai_agents_on_iterations  CHECK (max_iterations > 0 AND max_iterations <= 50),
			CONSTRAINT constraint_ai_agents_on_name_unique UNIQUE (name),
			CONSTRAINT constraint_ai_agents_on_slug_unique UNIQUE (slug)
		);
		CREATE INDEX IF NOT EXISTS index_ai_agents_on_enabled
			ON ai_agents(enabled) WHERE enabled = TRUE;
	`); err != nil {
		return err
	}

	// 2. ai_tools — tool definitions. type=go_builtin references a Go
	//    function by name in `implementation`; type=python_script embeds
	//    Python source directly in `implementation` (Enneo-style) and is
	//    dispatched through the F6 sandbox.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_tools (
			id                SERIAL PRIMARY KEY,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			name              TEXT NOT NULL,
			slug              TEXT NOT NULL,
			description       TEXT NULL,
			type              TEXT NOT NULL CHECK (type IN ('go_builtin','python_script')),
			category          TEXT NOT NULL CHECK (category IN ('read','write','decision','universal')),
			input_parameters  JSONB NOT NULL DEFAULT '[]'::jsonb,
			output_schema     JSONB NOT NULL DEFAULT '{}'::jsonb,
			implementation    JSONB NOT NULL DEFAULT '{}'::jsonb,
			min_legitimation  INT NOT NULL DEFAULT 0,
			timeout_ms        INT NOT NULL DEFAULT 30000,
			async             BOOL NOT NULL DEFAULT FALSE,
			enabled           BOOL NOT NULL DEFAULT TRUE,
			CONSTRAINT constraint_ai_tools_on_name        CHECK (length(name) <= 140),
			CONSTRAINT constraint_ai_tools_on_slug        CHECK (length(slug) <= 140),
			CONSTRAINT constraint_ai_tools_on_description CHECK (description IS NULL OR length(description) <= 1000),
			CONSTRAINT constraint_ai_tools_on_legit       CHECK (min_legitimation >= 0),
			CONSTRAINT constraint_ai_tools_on_timeout     CHECK (timeout_ms > 0 AND timeout_ms <= 600000),
			CONSTRAINT constraint_ai_tools_on_name_unique UNIQUE (name),
			CONSTRAINT constraint_ai_tools_on_slug_unique UNIQUE (slug)
		);
		CREATE INDEX IF NOT EXISTS index_ai_tools_on_enabled
			ON ai_tools(enabled) WHERE enabled = TRUE;
		CREATE INDEX IF NOT EXISTS index_ai_tools_on_type
			ON ai_tools(type);
	`); err != nil {
		return err
	}

	// 3. Grant the new permissions to the seeded Admin role. Mirrors the
	//    v2_5_0 pattern; idempotent via NOT (... = ANY(permissions)).
	for _, perm := range []string{"ai_agents:manage", "ai_tools:manage", "ai_tools:invoke"} {
		if _, err := db.Exec(`
			UPDATE roles
			SET    permissions = permissions || ARRAY[$1]::TEXT[]
			WHERE  name = 'Admin'
			  AND  NOT ($1 = ANY(permissions));
		`, perm); err != nil {
			return err
		}
	}

	// 4. Seed one stub agent and one universal Go-builtin tool so a fresh
	//    DB has something real on day one. Both seeds are admin-visible
	//    and edit-able but not wired to any runtime yet:
	//      - agent: enabled=false (won't fire on any tickets — admins
	//        flip it on after authoring a real prompt + detection)
	//      - tool: type=go_builtin, function="KBSearch" (placeholder; the
	//        agent runtime will register the real KBSearch Go function
	//        when F7+pipeline land — see TODO in cmd/ai_tools.go).
	//    ON CONFLICT DO NOTHING keeps the upgrade path idempotent for
	//    DBs that already have these slugs (e.g., from manual seeding).
	if _, err := db.Exec(`
		INSERT INTO ai_agents (
			name, slug, description, prompt, detection, tools, kb_folder_ids,
			model_preset, confidence_threshold, max_iterations, autonomy, enabled
		) VALUES (
			'Test Agent',
			'test-agent',
			'Stub agent — disabled. Edit prompt and enable when ready.',
			'',
			'{}'::jsonb,
			'[]'::jsonb,
			ARRAY[]::BIGINT[],
			'default',
			0.5,
			5,
			'{}'::jsonb,
			FALSE
		) ON CONFLICT (slug) DO NOTHING;
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT INTO ai_tools (
			name, slug, description, type, category,
			input_parameters, output_schema, implementation,
			min_legitimation, timeout_ms, async, enabled
		) VALUES (
			'kb_search_universal',
			'kb-search-universal',
			'Universal knowledge-base search. STUB — wire to real KB search Go function when agent runtime lands.',
			'go_builtin',
			'universal',
			'[{"key":"query","name":"Search query","type":"STRING","source":"manual","required":true}]'::jsonb,
			'{"type":"object","properties":{"results":{"type":"array"}}}'::jsonb,
			'{"function":"KBSearch"}'::jsonb,
			0,
			15000,
			FALSE,
			TRUE
		) ON CONFLICT (slug) DO NOTHING;
	`); err != nil {
		return err
	}

	return nil
}
