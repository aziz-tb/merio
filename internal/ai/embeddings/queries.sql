-- name: semantic-search-chunks
-- Inputs:
--   $1 query_vector  halfvec(1024)
--   $2 content_type  TEXT  (e.g. 'kb_chunk')
--   $3 folder_ids    BIGINT[]  (NULL or empty = no folder filter)
--   $4 limit         INT
--
-- Joins each candidate chunk to its source article's active version
-- (articles_index_state.active_version) so superseded vectors don't
-- leak into results during a re-indexing migration. Status filter is
-- 'published' only — drafts never surface to agents.
WITH active_versions AS (
    SELECT article_id, active_version
    FROM articles_index_state
)
SELECT
    e.id              AS embedding_id,
    e.source_id       AS article_id,
    e.chunk_index     AS chunk_index,
    e.chunk_text      AS chunk_text,
    e.source_path     AS source_path,
    a.title           AS title,
    a.folder_id       AS folder_id,
    1 - (e.embedding <=> $1) AS similarity
FROM embeddings e
JOIN active_versions av
    ON av.article_id = e.source_id
    AND av.active_version = e.source_version
JOIN kb_articles a ON a.id = e.source_id
WHERE e.content_type = $2
    AND ($3::BIGINT[] IS NULL OR cardinality($3) = 0 OR a.folder_id = ANY($3))
    AND a.status = 'published'
ORDER BY e.embedding <=> $1
LIMIT $4;

-- name: keyword-search-articles
-- Inputs:
--   $1 query_text   TEXT
--   $2 folder_ids   BIGINT[]  (NULL or empty = no folder filter)
--   $3 locale       TEXT  (selects the tsquery config to match the
--                          generated tsvector column's per-locale branch)
--   $4 limit        INT
--
-- Uses websearch_to_tsquery — same syntax users naturally type. The
-- query config (german/english/simple) must match the article's
-- tsvector config or ranking falls back to lexical exact-match only.
-- We default to 'simple' for any locale we don't have a built-in
-- dictionary for; same fallback as the generated column.
SELECT
    a.id                            AS article_id,
    a.title                         AS title,
    a.folder_id                     AS folder_id,
    substring(a.body_markdown, 1, 200) AS body_excerpt,
    ts_rank_cd(
        a.body_tsvector,
        websearch_to_tsquery(
            (CASE $3 WHEN 'de' THEN 'german' WHEN 'en' THEN 'english' ELSE 'simple' END)::regconfig,
            $1
        )
    ) AS rank
FROM kb_articles a
WHERE a.body_tsvector @@ websearch_to_tsquery(
        (CASE $3 WHEN 'de' THEN 'german' WHEN 'en' THEN 'english' ELSE 'simple' END)::regconfig,
        $1
    )
    AND a.status = 'published'
    AND ($2::BIGINT[] IS NULL OR cardinality($2) = 0 OR a.folder_id = ANY($2))
ORDER BY rank DESC
LIMIT $4;

-- name: insert-embedding
-- ON CONFLICT DO NOTHING: same (content,source,version,chunk,model)
-- re-embedded is a no-op. The catch-up sweep + workflow retries lean
-- on this for idempotency.
INSERT INTO embeddings (
    content_type, source_id, source_version, chunk_index,
    chunk_text, content_hash, embedding_model, embedding, source_path
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (content_type, source_id, source_version, chunk_index, embedding_model)
DO NOTHING;

-- name: upsert-articles-index-state
-- Atomic active-version swap. Called after StoreEmbeddings has
-- inserted the new-version rows. The hybrid-search semantic JOIN reads
-- only rows matching this row's active_version, so flipping this row
-- atomically transitions the article to the new version.
INSERT INTO articles_index_state (article_id, active_version, active_model, last_indexed_at, is_stale)
VALUES ($1, $2, $3, NOW(), FALSE)
ON CONFLICT (article_id) DO UPDATE
SET active_version  = EXCLUDED.active_version,
    active_model    = EXCLUDED.active_model,
    last_indexed_at = NOW(),
    is_stale        = FALSE;

-- name: get-articles-index-state
SELECT article_id, active_version, active_model, last_indexed_at, is_stale
FROM articles_index_state
WHERE article_id = $1;

-- name: delete-embeddings-for-version
-- Used by the cleanup workflow after the 7-day grace period.
DELETE FROM embeddings
WHERE content_type = $1
  AND source_id = $2
  AND source_version = $3;

-- name: delete-embeddings-for-article
-- Used on Unpublish / DeleteArticle. Drops every version's vectors.
DELETE FROM embeddings
WHERE content_type = 'kb_chunk' AND source_id = $1;

-- name: delete-articles-index-state-for-article
DELETE FROM articles_index_state WHERE article_id = $1;

-- name: list-stale-article-ids
-- Catch-up sweep: articles marked stale by the save-handler fallback.
SELECT article_id
FROM articles_index_state
WHERE is_stale = TRUE
ORDER BY last_indexed_at ASC
LIMIT $1;

-- name: list-published-article-ids-for-reindex
-- Bulk reindex driver: all published articles, paged.
SELECT id
FROM kb_articles
WHERE status = 'published'
ORDER BY id ASC
LIMIT $1 OFFSET $2;
