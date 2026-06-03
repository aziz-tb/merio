package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V0_2_0 adds presentation metadata for conversation statuses.
func V0_2_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE conversation_statuses
			ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT 'blue',
			ADD COLUMN IF NOT EXISTS icon TEXT NOT NULL DEFAULT 'CircleDot';

		UPDATE conversation_statuses
		SET color = CASE name
				WHEN 'Open' THEN 'blue'
				WHEN 'Snoozed' THEN 'yellow'
				WHEN 'Resolved' THEN 'green'
				WHEN 'Closed' THEN 'grey'
				ELSE CASE category
					WHEN 'waiting' THEN 'yellow'
					WHEN 'resolved' THEN 'green'
					ELSE 'blue'
				END
			END,
			icon = CASE name
				WHEN 'Open' THEN 'CircleDot'
				WHEN 'Snoozed' THEN 'Moon'
				WHEN 'Resolved' THEN 'CircleCheck'
				WHEN 'Closed' THEN 'CircleSlash'
				ELSE CASE category
					WHEN 'waiting' THEN 'Moon'
					WHEN 'resolved' THEN 'CircleCheck'
					ELSE 'CircleDot'
				END
			END
		WHERE name IN ('Open', 'Snoozed', 'Resolved', 'Closed')
			OR (color = 'blue' AND icon = 'CircleDot');
	`)
	return err
}
