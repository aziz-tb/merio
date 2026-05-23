-- name: insert-folder
INSERT INTO
    kb_folders (parent_id, name, slug)
VALUES
    ($1, $2, $3)
RETURNING *;

-- name: update-folder
UPDATE
    kb_folders
SET
    name = $2,
    updated_at = NOW()
WHERE
    id = $1
RETURNING *;

-- name: move-folder
UPDATE
    kb_folders
SET
    parent_id = $2,
    updated_at = NOW()
WHERE
    id = $1
RETURNING *;

-- name: delete-folder
DELETE FROM
    kb_folders
WHERE
    id = $1;

-- name: get-folder
SELECT
    id, parent_id, name, slug, sort_order, created_at, updated_at
FROM
    kb_folders
WHERE
    id = $1;

-- name: list-folders-by-parent
-- $1 is null.Int; IS NOT DISTINCT FROM treats NULL = NULL as a match,
-- so callers pass null.Int{Valid:false} to fetch root-level folders.
SELECT
    id, parent_id, name, slug, sort_order, created_at, updated_at
FROM
    kb_folders
WHERE
    parent_id IS NOT DISTINCT FROM $1
ORDER BY
    created_at ASC;

-- name: list-all-folders
SELECT
    id, parent_id, name, slug, sort_order, created_at, updated_at
FROM
    kb_folders
ORDER BY
    created_at ASC;

-- name: get-folder-descendants
-- Recursive CTE returning the IDs of every transitive descendant of $1.
-- Used by MoveFolder to reject cycles before issuing the UPDATE.
WITH RECURSIVE descendants AS (
    SELECT id
    FROM   kb_folders
    WHERE  parent_id = $1
    UNION
    SELECT f.id
    FROM   kb_folders f
    INNER JOIN descendants d ON f.parent_id = d.id
)
SELECT id FROM descendants;

-- name: insert-article
INSERT INTO
    kb_articles (
        folder_id, slug, title, locale, classification,
        is_ai_groundable, body_html, body_tiptap_json, body_markdown,
        author_user_id
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: update-article
-- Bumps revision unconditionally so the embed workflow ID
-- (embed-article:<id>:v<revision>) deduplicates by content state.
UPDATE
    kb_articles
SET
    slug = $2,
    title = $3,
    locale = $4,
    classification = $5,
    is_ai_groundable = $6,
    body_html = $7,
    body_tiptap_json = $8,
    body_markdown = $9,
    revision = revision + 1,
    updated_at = NOW()
WHERE
    id = $1
RETURNING *;

-- name: publish-article
-- Refuses to flip an already-published row by gating on the current status;
-- callers receive sql.ErrNoRows when the precondition fails. Bumps
-- revision so the publish event has its own workflow ID distinct from
-- the prior draft revision (in case a draft was embedded for previews).
UPDATE
    kb_articles
SET
    status = 'published',
    published_at = NOW(),
    revision = revision + 1,
    updated_at = NOW()
WHERE
    id = $1
    AND status = 'draft'
RETURNING *;

-- name: unpublish-article
-- Symmetric guard: fails when the row is already a draft. Revision is
-- NOT bumped on unpublish — the content didn't change, only its
-- visibility. The trigger handler deletes active vectors instead.
UPDATE
    kb_articles
SET
    status = 'draft',
    published_at = NULL,
    updated_at = NOW()
WHERE
    id = $1
    AND status = 'published'
RETURNING *;

-- name: mark-index-stale
-- Best-effort upsert on articles_index_state.is_stale = TRUE. Called
-- as a fallback when workflow start fails so the catch-up sweep can
-- eventually re-embed. Inserts a placeholder row if none exists yet
-- (first-save case) so the stale flag has somewhere to live.
INSERT INTO articles_index_state (article_id, active_version, active_model, is_stale)
VALUES ($1, 1, '', TRUE)
ON CONFLICT (article_id) DO UPDATE
SET is_stale = TRUE;

-- name: get-article-revision
SELECT revision FROM kb_articles WHERE id = $1;

-- name: move-article
UPDATE
    kb_articles
SET
    folder_id = $2,
    updated_at = NOW()
WHERE
    id = $1
RETURNING *;

-- name: delete-article
DELETE FROM
    kb_articles
WHERE
    id = $1;

-- name: get-article
SELECT
    id, folder_id, slug, title, locale, classification, status,
    is_ai_groundable, body_html, body_tiptap_json, body_markdown,
    author_user_id, published_at, created_at, updated_at
FROM
    kb_articles
WHERE
    id = $1;

-- name: list-articles
-- $1 folder_id        BIGINT   (NULL = any)
-- $2 classifications  TEXT[]   (cardinality 0 = any)
-- $3 statuses         TEXT[]   (cardinality 0 = any)
-- $4 locale           TEXT     (NULL = any)
-- $5 search           TEXT     (NULL = any; ILIKE on title only — no body, no FTS)
-- $6 limit            INT
-- $7 offset           INT
SELECT
    id, folder_id, slug, title, locale, classification, status,
    is_ai_groundable, body_html, body_tiptap_json, body_markdown,
    author_user_id, published_at, created_at, updated_at
FROM
    kb_articles
WHERE
    ($1::BIGINT IS NULL OR folder_id = $1)
    AND ($2::TEXT[] IS NULL OR cardinality($2) = 0 OR classification = ANY($2))
    AND ($3::TEXT[] IS NULL OR cardinality($3) = 0 OR status = ANY($3))
    AND ($4::TEXT IS NULL OR locale = $4)
    AND ($5::TEXT IS NULL OR title ILIKE '%' || $5 || '%')
ORDER BY
    updated_at DESC
LIMIT $6 OFFSET $7;
