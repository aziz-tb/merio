package embeddings

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"github.com/zerodha/logf"

	"github.com/abhinavxd/libredesk/internal/ai/embeddings/chunking"
	"github.com/abhinavxd/libredesk/internal/dbutil"
)

//go:embed queries.sql
var efs embed.FS

// Store is the DB-side surface for embeddings: insert, query, swap
// active version, delete. Searcher composes a Store with a Provider +
// reranker to build the hybrid pipeline; activities compose a Store
// with the provider to write embeddings during indexing.
type Store struct {
	db *sqlx.DB
	q  storeQueries
	lo *logf.Logger
}

type storeQueries struct {
	SemanticSearchChunks         *sqlx.Stmt `query:"semantic-search-chunks"`
	KeywordSearchArticles        *sqlx.Stmt `query:"keyword-search-articles"`
	InsertEmbedding              *sqlx.Stmt `query:"insert-embedding"`
	UpsertArticlesIndexState     *sqlx.Stmt `query:"upsert-articles-index-state"`
	GetArticlesIndexState        *sqlx.Stmt `query:"get-articles-index-state"`
	DeleteEmbeddingsForVersion   *sqlx.Stmt `query:"delete-embeddings-for-version"`
	DeleteEmbeddingsForArticle   *sqlx.Stmt `query:"delete-embeddings-for-article"`
	DeleteArticlesIndexState     *sqlx.Stmt `query:"delete-articles-index-state-for-article"`
	ListStaleArticleIDs          *sqlx.Stmt `query:"list-stale-article-ids"`
	ListPublishedArticlesReindex *sqlx.Stmt `query:"list-published-article-ids-for-reindex"`
}

// StoreOpts wires dependencies.
type StoreOpts struct {
	DB *sqlx.DB
	Lo *logf.Logger
}

// NewStore prepares the embedded queries against db.
func NewStore(opt StoreOpts) (*Store, error) {
	if opt.DB == nil {
		return nil, errors.New("embeddings.Store: DB is required")
	}
	if opt.Lo == nil {
		return nil, errors.New("embeddings.Store: Lo is required")
	}
	var q storeQueries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, fmt.Errorf("embeddings.Store: prepare queries: %w", err)
	}
	return &Store{db: opt.DB, q: q, lo: opt.Lo}, nil
}

// StoredChunk is one chunk + its embedding, ready to land in the
// embeddings table.
type StoredChunk struct {
	Chunk     chunking.Chunk
	Embedding []float32
}

// StoreEmbeddings inserts a batch of chunks under (contentType, sourceID,
// sourceVersion). ON CONFLICT DO NOTHING handles idempotent retries:
// the same chunk with the same model is a no-op.
func (s *Store) StoreEmbeddings(
	ctx context.Context,
	contentType string,
	sourceID int64,
	sourceVersion int,
	embeddingModel string,
	chunks []StoredChunk,
) error {
	for _, c := range chunks {
		vec := pgvector.NewHalfVector(c.Embedding)
		_, err := s.q.InsertEmbedding.ExecContext(ctx,
			contentType,
			sourceID,
			sourceVersion,
			c.Chunk.Index,
			c.Chunk.Text,
			c.Chunk.ContentHash,
			embeddingModel,
			vec,
			c.Chunk.SourcePath,
		)
		if err != nil {
			return fmt.Errorf("embeddings.Store: insert chunk %d: %w", c.Chunk.Index, err)
		}
	}
	return nil
}

// SwapActiveVersion is the atomic version flip — once this UPDATE
// commits, the hybrid search starts seeing the new vectors. The old
// version's rows stay in the table until CleanupOldVectorsWorkflow
// deletes them after the 7-day grace.
func (s *Store) SwapActiveVersion(ctx context.Context, articleID int64, newVersion int, model string) error {
	_, err := s.q.UpsertArticlesIndexState.ExecContext(ctx, articleID, newVersion, model)
	if err != nil {
		return fmt.Errorf("embeddings.Store: swap active version: %w", err)
	}
	return nil
}

// IndexState is the row returned by GetIndexState.
type IndexState struct {
	ArticleID     int64  `db:"article_id"`
	ActiveVersion int    `db:"active_version"`
	ActiveModel   string `db:"active_model"`
	LastIndexed   string `db:"last_indexed_at"`
	IsStale       bool   `db:"is_stale"`
}

// GetIndexState fetches the active-version pointer for one article.
// Returns sql.ErrNoRows when the article has never been indexed.
func (s *Store) GetIndexState(ctx context.Context, articleID int64) (*IndexState, error) {
	var st IndexState
	if err := s.q.GetArticlesIndexState.GetContext(ctx, &st, articleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("embeddings.Store: get index state: %w", err)
	}
	return &st, nil
}

// DeleteVersion drops every chunk row for (contentType, sourceID,
// sourceVersion). Used by the cleanup workflow after the grace period.
func (s *Store) DeleteVersion(ctx context.Context, contentType string, sourceID int64, version int) error {
	_, err := s.q.DeleteEmbeddingsForVersion.ExecContext(ctx, contentType, sourceID, version)
	if err != nil {
		return fmt.Errorf("embeddings.Store: delete version: %w", err)
	}
	return nil
}

// DeleteArticle removes every vector for an article and its index-state
// row. Used on Unpublish / hard delete.
func (s *Store) DeleteArticle(ctx context.Context, articleID int64) error {
	if _, err := s.q.DeleteEmbeddingsForArticle.ExecContext(ctx, articleID); err != nil {
		return fmt.Errorf("embeddings.Store: delete article vectors: %w", err)
	}
	if _, err := s.q.DeleteArticlesIndexState.ExecContext(ctx, articleID); err != nil {
		return fmt.Errorf("embeddings.Store: delete article index state: %w", err)
	}
	return nil
}

// ListStaleArticleIDs returns article ids needing re-embed. Used by
// the catch-up sweep (when implemented).
func (s *Store) ListStaleArticleIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var ids []int64
	if err := s.q.ListStaleArticleIDs.SelectContext(ctx, &ids, limit); err != nil {
		return nil, fmt.Errorf("embeddings.Store: list stale: %w", err)
	}
	return ids, nil
}

// ListPublishedForReindex returns one page of published article ids in
// id order. Used by BulkReindexWorkflow.
func (s *Store) ListPublishedForReindex(ctx context.Context, limit, offset int) ([]int64, error) {
	if limit <= 0 {
		limit = 100
	}
	var ids []int64
	if err := s.q.ListPublishedArticlesReindex.SelectContext(ctx, &ids, limit, offset); err != nil {
		return nil, fmt.Errorf("embeddings.Store: list published: %w", err)
	}
	return ids, nil
}

// candidateRow is the shared shape used by both halves of the hybrid
// search before fusion.
type candidateRow struct {
	EmbeddingID int64   `db:"embedding_id"`
	ArticleID   int64   `db:"article_id"`
	ChunkIndex  int     `db:"chunk_index"`
	ChunkText   string  `db:"chunk_text"`
	SourcePath  string  `db:"source_path"`
	Title       string  `db:"title"`
	FolderID    int64   `db:"folder_id"`
	Similarity  float64 `db:"similarity"`
}

type keywordRow struct {
	ArticleID   int64   `db:"article_id"`
	Title       string  `db:"title"`
	FolderID    int64   `db:"folder_id"`
	BodyExcerpt string  `db:"body_excerpt"`
	Rank        float64 `db:"rank"`
}

// semanticSearch runs the chunk-level vector similarity query. Returns
// up to `limit` rows ordered by cosine distance (ascending) — closest
// first. folderIDs nil/empty = no folder filter.
func (s *Store) semanticSearch(
	ctx context.Context,
	queryVec []float32,
	contentType string,
	folderIDs []int64,
	limit int,
) ([]candidateRow, error) {
	var folderArr any
	if len(folderIDs) > 0 {
		folderArr = pq.Int64Array(folderIDs)
	} else {
		folderArr = pq.Int64Array(nil)
	}
	var rows []candidateRow
	vec := pgvector.NewHalfVector(queryVec)
	if err := s.q.SemanticSearchChunks.SelectContext(ctx, &rows, vec, contentType, folderArr, limit); err != nil {
		return nil, fmt.Errorf("embeddings.Store: semantic search: %w", err)
	}
	return rows, nil
}

// keywordSearch runs the article-level tsvector query. Locale picks
// the matching tsquery config.
func (s *Store) keywordSearch(
	ctx context.Context,
	query string,
	folderIDs []int64,
	locale string,
	limit int,
) ([]keywordRow, error) {
	var folderArr any
	if len(folderIDs) > 0 {
		folderArr = pq.Int64Array(folderIDs)
	} else {
		folderArr = pq.Int64Array(nil)
	}
	if locale == "" {
		locale = "simple"
	}
	var rows []keywordRow
	if err := s.q.KeywordSearchArticles.SelectContext(ctx, &rows, query, folderArr, locale, limit); err != nil {
		return nil, fmt.Errorf("embeddings.Store: keyword search: %w", err)
	}
	return rows, nil
}
