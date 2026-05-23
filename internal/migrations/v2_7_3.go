package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_3 migrates the ai_agents.tools JSONB shape from the F9 form to
// the F5-unified form (chunk 9 of the F7+F5 batch):
//
//   F9 shape:  [{tool_id: 12, config_override?: {...}}]
//   F5 shape:  [{kind: "internal"|"custom", id: <int|string>, config_override?: {...}}]
//
// Internal capabilities live in code (kb_search etc.) and are referenced
// by string id; custom tools live in ai_tools and are referenced by their
// numeric id. The unified resolver dispatches by `kind`.
//
// Conversion rules:
//   - tools is empty array → leave as []
//   - element has `kind` already → leave alone (idempotency: re-running
//     the migration on a converted DB is a no-op for that row)
//   - element has tool_id → rewrite as {kind: "custom", id: <tool_id>,
//     config_override: <existing>}
//   - any other shape → leave alone (defensive; surfaces in app-side
//     validation if it ever boots)
//
// The conversion runs as a single SQL UPDATE using jsonb_path_query +
// jsonb_array_elements + a CASE expression so it's atomic per row and
// safe to interrupt.
//
// Idempotent. Forward-only. Rollback is a manual SQL pass that maps the
// reverse direction (custom-only); not provided here because the
// downstream code wouldn't read the F9 shape anyway.
func V2_7_3(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// Migrate every ai_agents row whose tools array contains at least one
	// element WITHOUT a `kind` key. Rows that are already converted (every
	// element has `kind`) are skipped.
	const migrate = `
		UPDATE ai_agents
		SET    tools = sub.new_tools,
		       updated_at = NOW()
		FROM (
			SELECT id,
			       COALESCE(
			           jsonb_agg(
			               CASE
			                   WHEN elem ? 'kind' THEN elem
			                   WHEN elem ? 'tool_id' THEN
			                       jsonb_build_object(
			                           'kind', 'custom',
			                           'id',   (elem->>'tool_id')::INT
			                       )
			                       || CASE
			                              WHEN elem ? 'config_override'
			                              THEN jsonb_build_object('config_override', elem->'config_override')
			                              ELSE '{}'::jsonb
			                          END
			                   ELSE elem
			               END
			           ),
			           '[]'::jsonb
			       ) AS new_tools
			FROM   ai_agents,
			       LATERAL jsonb_array_elements(tools) AS elem
			WHERE  jsonb_typeof(tools) = 'array'
			  AND  jsonb_array_length(tools) > 0
			GROUP BY id
			HAVING bool_or(NOT (elem ? 'kind'))
		) AS sub
		WHERE ai_agents.id = sub.id;
	`
	if _, err := db.Exec(migrate); err != nil {
		return err
	}
	return nil
}
