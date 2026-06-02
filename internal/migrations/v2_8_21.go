package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_21 applies final customer-identification naming cleanup:
// legitimation -> verification, remaps verification levels from
// 0/10/20/30 to 0/1/2/3, and drops the retired functions.enabled column.
func V2_8_21(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE functions DROP COLUMN IF EXISTS enabled;

		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'conversations' AND column_name = 'legitimation_level'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'conversations' AND column_name = 'verification_level'
			) THEN
				ALTER TABLE conversations RENAME COLUMN legitimation_level TO verification_level;
			END IF;

			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'conversations' AND column_name = 'legitimation_message'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'conversations' AND column_name = 'verification_message'
			) THEN
				ALTER TABLE conversations RENAME COLUMN legitimation_message TO verification_message;
			END IF;

			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'ai_agents' AND column_name = 'min_legitimation'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'ai_agents' AND column_name = 'min_verification'
			) THEN
				ALTER TABLE ai_agents RENAME COLUMN min_legitimation TO min_verification;
			END IF;
		END $$;

		UPDATE conversations
		SET verification_level = CASE verification_level
			WHEN 10 THEN 1
			WHEN 20 THEN 2
			WHEN 30 THEN 3
			ELSE verification_level
		END
		WHERE verification_level IN (10, 20, 30);

		UPDATE ai_agents
		SET min_verification = CASE min_verification
			WHEN 10 THEN 1
			WHEN 20 THEN 2
			WHEN 30 THEN 3
			ELSE min_verification
		END
		WHERE min_verification IN (10, 20, 30);

		ALTER TABLE ai_agents
			DROP CONSTRAINT IF EXISTS constraint_ai_agents_on_min_legitimation;
		ALTER TABLE ai_agents
			DROP CONSTRAINT IF EXISTS constraint_ai_agents_on_min_verification;
		ALTER TABLE ai_agents
			ADD CONSTRAINT constraint_ai_agents_on_min_verification
			CHECK (min_verification >= 0 AND min_verification <= 3);

		UPDATE settings
		SET key = 'customer_identification.verification_rules_async'
		WHERE key = 'customer_identification.legitimation_rules_async'
		  AND NOT EXISTS (
			SELECT 1 FROM settings WHERE key = 'customer_identification.verification_rules_async'
		  );

		DELETE FROM settings
		WHERE key = 'customer_identification.legitimation_rules_async'
		  AND EXISTS (
			SELECT 1 FROM settings WHERE key = 'customer_identification.verification_rules_async'
		  );
	`)
	return err
}
