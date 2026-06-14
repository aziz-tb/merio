package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_9_0 adds the Composer capability section (Slice 5, §2/§7) — the general KB
// scope + general tool slugs the Composer uses to answer the general tier
// directly — and seeds one default verification guidance item so an upgraded
// install handles unverified customers out of the box.
//
// The seed is ordinary config (operator-editable/deletable), applied ONLY when
// guidance is still the empty default '[]', so an install that has already
// configured guidance is left untouched. Forward-only and idempotent (ADD COLUMN
// IF NOT EXISTS + the guidance='[]' guard).
func V0_9_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE composer_config
			ADD COLUMN IF NOT EXISTS capability JSONB NOT NULL
			DEFAULT '{"kb_scope":{"folder_ids":[],"article_ids":[]},"tool_slugs":[]}'::jsonb;

		UPDATE composer_config
		SET    guidance = '[{"id":"verification-default","title":"Ask unverified customers to verify before account actions","group":"other","body_text":"If the customer is not verified and asks about or requests an action on their own account (billing, contract, personal data, or account changes), ask them to complete identity verification before acting on it. General questions — opening hours, how-to, product info — do not require verification.","inbox_scope":[],"conditions":{"op":"AND","children":[{"leaf":{"source":"ticket","path":"verification_level","operator":"<","value":2}}]},"status":"live"}]'::jsonb
		WHERE  id = 1 AND guidance = '[]'::jsonb;
	`)
	return err
}
