package migrations

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_7_8 lands the F12 RAG foundation: pgvector extension, the universal
// `embeddings` table (halfvec(1024) + HNSW index), `articles_index_state`
// (active-version pointer + stale flag), an `is_stale`-partial supporting
// index for catch-up sweeps, a locale-aware `body_tsvector` generated
// column on kb_articles for the keyword half of hybrid search, and a
// `revision` column on kb_articles that the workflow ID format
// `embed-article:<id>:v<revision>` depends on for idempotency.
//
// Design notes lifted from the F12 plan + Phase 1 discovery:
//
//   - source_id and article_id use BIGINT to match kb_articles.id
//     (BIGSERIAL). The plan's INT was a discovery oversight.
//   - HNSW with halfvec_cosine_ops at default tuning (m=16,
//     ef_construction=64). ef_search is a per-query session knob; tuned
//     after real query patterns surface.
//   - tsvector generated column branches on locale so EN/FR articles
//     ship without a schema follow-up. 'simple' (no stemming) covers
//     locales we don't have built-in dictionaries for.
//   - revision starts at 1 and is bumped in Go (knowledge.Manager
//     UpdateArticle / PublishArticle). A trigger would couple migration
//     logic to application semantics; the Go bump keeps the contract in
//     one place.
//   - is_ai_groundable stays in the schema unchanged — the indexing
//     path stops checking it (per fork-5 resolution) so existing data
//     remains valid; the column can be dropped in a later migration.
//
// Forward-only. Idempotent via IF NOT EXISTS / ADD COLUMN IF NOT
// EXISTS. Rollback (manual, last resort):
//
//	BEGIN;
//	ALTER TABLE kb_articles DROP COLUMN IF EXISTS body_tsvector;
//	ALTER TABLE kb_articles DROP COLUMN IF EXISTS revision;
//	DROP TABLE IF EXISTS articles_index_state CASCADE;
//	DROP TABLE IF EXISTS embeddings CASCADE;
//	-- DROP EXTENSION vector;  -- only if no other table depends on it
//	COMMIT;
func V2_7_8(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// 1. Extension. pgvector/pgvector:pg17 image preinstalls the
	//    package; CREATE EXTENSION wires it into this DB.
	if _, err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("v2_7_8: create extension vector (is the Postgres image pgvector/pgvector:pg17?): %w", err)
	}

	// 2. Universal embeddings table. content_type is open-ended (TEXT)
	//    with a CHECK so the v1 set is enforced but adding a new
	//    content type later is a one-line ALTER. source_id is BIGINT
	//    so it can FK to kb_articles.id / conversations.id /
	//    customer_notes.id without truncation.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS embeddings (
			id              BIGSERIAL PRIMARY KEY,
			content_type    TEXT NOT NULL,
			source_id       BIGINT NOT NULL,
			source_version  INT NOT NULL DEFAULT 1,
			chunk_index     INT NOT NULL DEFAULT 0,
			chunk_text      TEXT NOT NULL,
			content_hash    TEXT NOT NULL,
			embedding_model TEXT NOT NULL,
			embedding       halfvec(1024) NOT NULL,
			source_path     TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT constraint_embeddings_on_content_type
				CHECK (content_type IN ('kb_chunk', 'ticket', 'customer_note')),
			CONSTRAINT constraint_embeddings_on_source_version
				CHECK (source_version > 0),
			CONSTRAINT constraint_embeddings_on_chunk_index
				CHECK (chunk_index >= 0)
		)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create embeddings: %w", err)
	}

	// 3. Idempotency unique index. Same (content,source,version,chunk)
	//    re-embedded with the same model is a no-op (different model
	//    or different version = different row, intentional). The
	//    StoreEmbeddings activity uses ON CONFLICT DO NOTHING against
	//    this index.
	if _, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS index_embeddings_on_content_source_version_chunk_model
			ON embeddings (content_type, source_id, source_version, chunk_index, embedding_model)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create embeddings unique index: %w", err)
	}

	// 4. HNSW index for vector similarity. halfvec_cosine_ops is the
	//    operator class for half-precision vectors with cosine
	//    distance; matches the embedding column type. m and
	//    ef_construction at pgvector defaults — tuning is empirical
	//    once real query patterns surface (docs/rag.md).
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS index_embeddings_on_vector
			ON embeddings USING hnsw (embedding halfvec_cosine_ops)
			WITH (m = 16, ef_construction = 64)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create HNSW index: %w", err)
	}

	// 5. Per-source-version index for cleanup ("delete all chunks of
	//    article X version N") and for the active-version JOIN in the
	//    semantic-search query.
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS index_embeddings_on_content_source_version
			ON embeddings (content_type, source_id, source_version)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create source-version index: %w", err)
	}

	// 6. Per-model index for model-upgrade-time queries ("which
	//    embeddings still use the old model?").
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS index_embeddings_on_model_created
			ON embeddings (embedding_model, created_at DESC)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create model index: %w", err)
	}

	// 7. articles_index_state — per-article pointer to the currently
	//    active version. The hybrid-search semantic JOIN reads only
	//    rows whose source_version matches the active version, so
	//    version swap is an atomic UPDATE.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS articles_index_state (
			article_id       BIGINT PRIMARY KEY
				REFERENCES kb_articles(id) ON DELETE CASCADE,
			active_version   INT NOT NULL DEFAULT 1,
			active_model     TEXT NOT NULL,
			last_indexed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			is_stale         BOOLEAN NOT NULL DEFAULT FALSE,
			CONSTRAINT constraint_articles_index_state_on_active_version
				CHECK (active_version > 0)
		)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create articles_index_state: %w", err)
	}

	// 8. Partial index on is_stale=TRUE for the catch-up sweep
	//    ("which articles still need re-indexing?"). Partial because
	//    99%+ of rows have is_stale=FALSE.
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS index_articles_index_state_on_stale
			ON articles_index_state (is_stale, last_indexed_at)
			WHERE is_stale = TRUE
	`); err != nil {
		return fmt.Errorf("v2_7_8: create stale index: %w", err)
	}

	// 9. revision column on kb_articles. Starts at 1; bumped in Go by
	//    knowledge.Manager.UpdateArticle / PublishArticle. Workflow IDs
	//    `embed-article:<id>:v<revision>` are deterministic and idempotent.
	if _, err := db.Exec(`
		ALTER TABLE kb_articles
			ADD COLUMN IF NOT EXISTS revision INT NOT NULL DEFAULT 1
	`); err != nil {
		return fmt.Errorf("v2_7_8: add revision column: %w", err)
	}

	// 10. Locale-aware tsvector generated column. Branches on locale so
	//     EN/FR/other articles ship with the right stemmer from day
	//     one. 'simple' (no stemming) is the fallback for locales we
	//     don't have a built-in dictionary for.
	if _, err := db.Exec(`
		ALTER TABLE kb_articles
			ADD COLUMN IF NOT EXISTS body_tsvector tsvector
			GENERATED ALWAYS AS (
				CASE locale
					WHEN 'de' THEN to_tsvector('german',
						coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
					WHEN 'en' THEN to_tsvector('english',
						coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
					ELSE to_tsvector('simple',
						coalesce(title, '') || ' ' || coalesce(body_markdown, ''))
				END
			) STORED
	`); err != nil {
		return fmt.Errorf("v2_7_8: add body_tsvector column: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS index_kb_articles_on_body_tsvector
			ON kb_articles USING GIN (body_tsvector)
	`); err != nil {
		return fmt.Errorf("v2_7_8: create tsvector GIN index: %w", err)
	}

	return nil
}
