package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_4 gives existing AI agents the structured detection scaffold expected
// by the visual DetectionBuilder. Dev one-shot: no schema change beyond the
// column default; existing {} rows become manual_only detections.
func V2_8_4(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		ALTER TABLE ai_agents
			ALTER COLUMN detection SET DEFAULT '{"mode":"manual_only","rules":{"op":"AND","children":[]},"ai":{"prompt":""}}'::jsonb;

		UPDATE ai_agents
		SET detection = '{"mode":"manual_only","rules":{"op":"AND","children":[]},"ai":{"prompt":""}}'::jsonb
		WHERE detection IS NULL OR detection = '{}'::jsonb;
	`); err != nil {
		return fmt.Errorf("v2_8_4: agent detection default: %w", err)
	}
	return nil
}
