package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_4_0 provisions the knowledge base tables (B3) and grants the new
// permissions to the existing seeded roles on upgraded instances. Idempotent.
//
// The libredesk migration framework is forward-only. To roll this back,
// run the SQL below manually in psql against the same database, then
// redeploy a build that no longer contains the knowledge code:
//
//	BEGIN;
//	UPDATE roles
//	SET    permissions = ARRAY(
//	           SELECT UNNEST(permissions)
//	           EXCEPT SELECT UNNEST(ARRAY[
//	               'knowledge:read',
//	               'knowledge:write',
//	               'knowledge:publish',
//	               'knowledge:delete'
//	           ])
//	       );
//	DROP INDEX IF EXISTS index_kb_articles_on_author_user_id;
//	DROP INDEX IF EXISTS index_kb_articles_on_status_classification_locale;
//	DROP TABLE IF EXISTS kb_articles;
//	DROP INDEX IF EXISTS kb_folders_root_slug_uniq;
//	DROP INDEX IF EXISTS kb_folders_parent_slug_uniq;
//	DROP TABLE IF EXISTS kb_folders;
//	COMMIT;
//
// Note: custom roles inheriting from Agent are NOT touched by the
// upgrade-time grant — operators with custom roles must edit them
// manually via the Roles admin UI.
func V2_4_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Tables and indexes. IF NOT EXISTS makes this safe on re-run.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kb_folders (
			id          BIGSERIAL PRIMARY KEY,
			parent_id   BIGINT NULL
			            REFERENCES kb_folders(id)
			            ON DELETE RESTRICT ON UPDATE CASCADE,
			name        VARCHAR(255) NOT NULL,
			slug        VARCHAR(255) NOT NULL,
			sort_order  INT          NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX IF NOT EXISTS kb_folders_parent_slug_uniq
			ON kb_folders (parent_id, slug)
			WHERE parent_id IS NOT NULL;

		CREATE UNIQUE INDEX IF NOT EXISTS kb_folders_root_slug_uniq
			ON kb_folders (slug)
			WHERE parent_id IS NULL;

		CREATE TABLE IF NOT EXISTS kb_articles (
			id                BIGSERIAL PRIMARY KEY,
			folder_id         BIGINT       NOT NULL
			                  REFERENCES kb_folders(id)
			                  ON DELETE RESTRICT ON UPDATE CASCADE,
			slug              VARCHAR(255) NOT NULL,
			title             VARCHAR(500) NOT NULL,
			locale            VARCHAR(8)   NOT NULL DEFAULT 'en',
			classification    VARCHAR(32)  NOT NULL,
			status            VARCHAR(32)  NOT NULL DEFAULT 'draft',
			is_ai_groundable  BOOLEAN      NOT NULL DEFAULT FALSE,
			body_html         TEXT         NOT NULL DEFAULT '',
			body_tiptap_json  JSONB        NOT NULL DEFAULT '{}'::jsonb,
			body_markdown     TEXT         NOT NULL DEFAULT '',
			author_user_id    BIGINT       NULL
			                  REFERENCES users(id) ON DELETE SET NULL,
			published_at      TIMESTAMPTZ  NULL,
			created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			CONSTRAINT constraint_kb_articles_on_classification
				CHECK (classification IN ('faq', 'how_to', 'policy', 'news', 'other')),
			CONSTRAINT constraint_kb_articles_on_status
				CHECK (status IN ('draft', 'published')),
			CONSTRAINT kb_articles_folder_slug_locale_uniq
				UNIQUE (folder_id, slug, locale)
		);

		CREATE INDEX IF NOT EXISTS index_kb_articles_on_status_classification_locale
			ON kb_articles (status, classification, locale);

		CREATE INDEX IF NOT EXISTS index_kb_articles_on_author_user_id
			ON kb_articles (author_user_id);
	`); err != nil {
		return err
	}

	// 2. Grant the new permissions on existing seeded roles. The schema.sql
	//    seed handles fresh installs; this branch handles upgraded instances.
	//    The NOT-contains guards keep this strictly idempotent. Style follows
	//    v2_3_0.go: one batched UPDATE per role, guard checks the last perm.
	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY['knowledge:read']::TEXT[]
		WHERE  name = 'Agent'
		  AND  NOT ('knowledge:read' = ANY(permissions));
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		UPDATE roles
		SET    permissions = permissions || ARRAY[
		           'knowledge:read',
		           'knowledge:write',
		           'knowledge:publish',
		           'knowledge:delete'
		       ]::TEXT[]
		WHERE  name = 'Admin'
		  AND  NOT ('knowledge:delete' = ANY(permissions));
	`); err != nil {
		return err
	}

	return nil
}
