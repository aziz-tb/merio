package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_9 adds a dispatch category to custom ai_tools rows.
//
// Built-in capabilities live in Go code and declare their category via
// builtin.InternalCapability.Category(). This column applies only to
// custom Python/JavaScript tools stored in ai_tools.
//
// The category controls same-turn parallel dispatch:
//   - read  → eligible for parallel execution within a turn
//   - write → runs sequentially after reads complete
//
// Existing rows default to "write" so operators must explicitly opt in
// after auditing their tool's behavior for side effects.
func V2_7_9(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		ALTER TABLE ai_tools
			ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'write'
	`); err != nil {
		return fmt.Errorf("v2_7_9: add ai_tools.category: %w", err)
	}

	if _, err := db.Exec(`
		UPDATE ai_tools
		SET    category = 'write'
		WHERE  category IS NULL
		    OR category NOT IN ('read', 'write')
	`); err != nil {
		return fmt.Errorf("v2_7_9: normalize ai_tools.category: %w", err)
	}

	if _, err := db.Exec(`
		ALTER TABLE ai_tools
			DROP CONSTRAINT IF EXISTS constraint_ai_tools_on_category
	`); err != nil {
		return fmt.Errorf("v2_7_9: drop ai_tools.category constraint: %w", err)
	}

	if _, err := db.Exec(`
		ALTER TABLE ai_tools
			ADD CONSTRAINT constraint_ai_tools_on_category
				CHECK (category IN ('read', 'write'))
	`); err != nil {
		return fmt.Errorf("v2_7_9: add ai_tools.category constraint: %w", err)
	}

	return nil
}
