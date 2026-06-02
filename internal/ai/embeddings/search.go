package embeddings

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zerodha/logf"

	"github.com/abhinavxd/libredesk/internal/ai/reranker"
)

const rerankerCandidateChunks = 3

// SearchInput is the unified query interface for hybrid search.
type SearchInput struct {
	// Query is the natural-language search string.
	Query string

	// ContentType selects which vectors to search. 'kb_chunk' for KB.
	ContentType string

	// FolderIDs filters article-level results to these folder ids.
	// Empty / nil = no folder filter. The caller (kb_search capability)
	// applies KB-scope semantics before passing.
	FolderIDs []int64

	// Locale picks the tsquery configuration for the keyword half.
	// Defaults to 'simple' (no stemming) when empty.
	Locale string

	// TopK caps the final result count (default 5).
	TopK int

	// RerankCandidates is the number of fused candidates passed to the
	// reranker (default 30). Lower = faster + cheaper but reduces
	// recall; higher = more accurate ranking but more reranker spend.
	RerankCandidates int

	// SemanticCandidates and KeywordCandidates control stage-1 fan-out
	// per half. Defaults to 30 each (the locked spec).
	SemanticCandidates int
	KeywordCandidates  int
}

// SearchResult is one article returned to the caller with retrieval
// metadata for observability + downstream prompt grounding.
type SearchResult struct {
	ArticleID     int64          `json:"article_id"`
	Title         string         `json:"title"`
	FolderID      int64          `json:"folder_id"`
	BodyExcerpt   string         `json:"body_excerpt,omitempty"`
	MatchedChunks []MatchedChunk `json:"matched_chunks"`

	// Scores — kept on the response for trace UIs + future ranking
	// analysis. Zero values mean "not matched in that stage."
	SemanticRank  int     `json:"semantic_rank,omitempty"`
	KeywordRank   int     `json:"keyword_rank,omitempty"`
	RRFScore      float64 `json:"rrf_score"`
	RerankerScore float64 `json:"reranker_score"`
}

// MatchedChunk is the per-chunk excerpt surfaced inside a SearchResult.
type MatchedChunk struct {
	ChunkIndex     int     `json:"chunk_index"`
	ChunkText      string  `json:"chunk_text"`
	ChunkChars     int     `json:"chunk_chars"`
	ChunkTruncated bool    `json:"chunk_truncated"`
	SourcePath     string  `json:"source_path,omitempty"`
	Similarity     float64 `json:"similarity,omitempty"`
}

// Searcher coordinates the three-stage hybrid pipeline. One per app —
// thread-safe across concurrent kb_search calls.
type Searcher struct {
	store                 *Store
	embed                 Provider
	reranker              reranker.Provider
	cache                 *QueryEmbeddingCache
	lo                    *logf.Logger
	minSemanticSimilarity float64
	minRerankerScore      float64
}

// SearcherOpts wires dependencies. The two threshold fields default to
// zero (no filter); callers are responsible for supplying tuned values.
// Use NewConfiguredSearcher when building from the shared app config.
type SearcherOpts struct {
	Store                 *Store
	Embed                 Provider
	Reranker              reranker.Provider
	Cache                 *QueryEmbeddingCache
	Lo                    *logf.Logger
	MinSemanticSimilarity float64
	MinRerankerScore      float64
}

// NewSearcher builds a Searcher. Reranker is optional (nil falls back
// to RRF order); Cache is optional (nil disables query embedding cache,
// every call hits the provider).
func NewSearcher(opt SearcherOpts) (*Searcher, error) {
	if opt.Store == nil {
		return nil, errors.New("embeddings.Searcher: Store is required")
	}
	if opt.Embed == nil {
		return nil, errors.New("embeddings.Searcher: Embed is required")
	}
	if opt.Lo == nil {
		return nil, errors.New("embeddings.Searcher: Lo is required")
	}
	return &Searcher{
		store:                 opt.Store,
		embed:                 opt.Embed,
		reranker:              opt.Reranker,
		cache:                 opt.Cache,
		lo:                    opt.Lo,
		minSemanticSimilarity: opt.MinSemanticSimilarity,
		minRerankerScore:      opt.MinRerankerScore,
	}, nil
}

// Search runs the three-stage hybrid pipeline.
//
// Stage 1: semantic (chunk-level cosine) + keyword (article-level
// tsvector + ts_rank_cd) in parallel.
// Stage 2: RRF fusion with k=60, deduplicated by article id.
// Stage 3: cross-encoder reranking of the top-RerankCandidates fused
// results down to TopK. Reranker failure falls back to RRF order.
func (s *Searcher) Search(ctx context.Context, in SearchInput) ([]SearchResult, error) {
	if in.TopK == 0 {
		in.TopK = 5
	}
	if in.RerankCandidates == 0 {
		in.RerankCandidates = 30
	}
	if in.SemanticCandidates == 0 {
		in.SemanticCandidates = 30
	}
	if in.KeywordCandidates == 0 {
		in.KeywordCandidates = 30
	}
	if in.ContentType == "" {
		in.ContentType = "kb_chunk"
	}

	// Stage 1 — both halves run in parallel; both queries hit Postgres
	// independently. Errors from either half fail the whole search; we
	// could fall back to single-half but that masks a real DB issue.
	queryVec, err := s.getOrEmbedQuery(ctx, in.Query)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: embed query: %w", err)
	}

	var (
		semantic []candidateRow
		keyword  []keywordRow
		semErr   error
		kwErr    error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		semantic, semErr = s.store.semanticSearch(ctx, queryVec, in.ContentType, in.FolderIDs, in.SemanticCandidates)
	}()
	go func() {
		defer wg.Done()
		keyword, kwErr = s.store.keywordSearch(ctx, in.Query, in.FolderIDs, in.Locale, in.KeywordCandidates)
	}()
	wg.Wait()
	if semErr != nil {
		return nil, fmt.Errorf("hybrid search: semantic: %w", semErr)
	}
	if kwErr != nil {
		return nil, fmt.Errorf("hybrid search: keyword: %w", kwErr)
	}

	if s.minSemanticSimilarity > 0 {
		semantic = filterChunksBySimilarity(semantic, s.minSemanticSimilarity)
	}

	// Stage 2 — RRF fusion + dedup by article id.
	fused := reciprocalRankFusion(semantic, keyword, 60)
	if len(fused) == 0 {
		return nil, nil
	}
	if len(fused) > in.RerankCandidates {
		fused = fused[:in.RerankCandidates]
	}

	// Stage 3 — cross-encoder reranking. Reranker is optional;
	// fallback to RRF order when nil or on failure.
	if s.reranker == nil {
		return fused[:min(in.TopK, len(fused))], nil
	}

	candidateTexts := make([]string, len(fused))
	for i, c := range fused {
		candidateTexts[i] = rerankerCandidateText(c)
	}
	rr, err := s.reranker.Rerank(ctx, in.Query, candidateTexts, in.TopK)
	if err != nil {
		s.lo.Warn("rerank failed; falling back to RRF order", "error", err)
		return fused[:min(in.TopK, len(fused))], nil
	}
	final := make([]SearchResult, 0, len(rr))
	for _, r := range rr {
		if r.Index < 0 || r.Index >= len(fused) {
			continue
		}
		fused[r.Index].RerankerScore = r.RelevanceScore
		final = append(final, fused[r.Index])
	}
	if s.minRerankerScore > 0 {
		final = filterResultsByReranker(final, s.minRerankerScore)
	}
	return final, nil
}

// filterChunksBySimilarity drops chunks whose cosine similarity is below
// the threshold. Used to keep weak retrieval candidates out of fusion +
// reranking. The kept order is preserved.
func filterChunksBySimilarity(chunks []candidateRow, min float64) []candidateRow {
	if min <= 0 || len(chunks) == 0 {
		return chunks
	}
	out := chunks[:0]
	for _, c := range chunks {
		if c.Similarity >= min {
			out = append(out, c)
		}
	}
	return out
}

// filterResultsByReranker drops articles whose reranker score is below
// the threshold. Applied after the cross-encoder pass. Preserves the
// caller-chosen ordering.
func filterResultsByReranker(results []SearchResult, min float64) []SearchResult {
	if min <= 0 || len(results) == 0 {
		return results
	}
	out := results[:0]
	for _, r := range results {
		if r.RerankerScore >= min {
			out = append(out, r)
		}
	}
	return out
}

// getOrEmbedQuery checks the Redis cache before calling the provider.
// Cache miss → embed → write back. Cache nil → always embed.
func (s *Searcher) getOrEmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if s.cache != nil {
		if vec, ok := s.cache.Get(ctx, query, s.embed.Model()); ok {
			return vec, nil
		}
	}
	vecs, err := s.embed.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, errors.New("embeddings.Searcher: provider returned zero vectors")
	}
	if s.cache != nil {
		s.cache.Set(ctx, query, s.embed.Model(), vecs[0])
	}
	return vecs[0], nil
}

// reciprocalRankFusion fuses semantic + keyword candidates into a
// per-article ranked list. k=60 is the standard RRF constant from
// Cormack et al; controls how steeply rank-1 dominates the score.
//
// Per-article aggregation: each article's matched chunks are
// accumulated (a multi-chunk match is signal). semantic_rank +
// keyword_rank are stamped for trace visibility.
func reciprocalRankFusion(semantic []candidateRow, keyword []keywordRow, k float64) []SearchResult {
	byArticle := make(map[int64]*SearchResult)
	for rank, c := range semantic {
		res, ok := byArticle[c.ArticleID]
		if !ok {
			res = &SearchResult{
				ArticleID: c.ArticleID,
				Title:     c.Title,
				FolderID:  c.FolderID,
			}
			byArticle[c.ArticleID] = res
		}
		res.RRFScore += 1.0 / (k + float64(rank+1))
		if res.SemanticRank == 0 {
			res.SemanticRank = rank + 1
		}
		res.MatchedChunks = append(res.MatchedChunks, MatchedChunk{
			ChunkIndex:     c.ChunkIndex,
			ChunkText:      c.ChunkText,
			ChunkChars:     utf8.RuneCountInString(c.ChunkText),
			ChunkTruncated: false,
			SourcePath:     c.SourcePath,
			Similarity:     c.Similarity,
		})
	}
	for rank, c := range keyword {
		res, ok := byArticle[c.ArticleID]
		if !ok {
			res = &SearchResult{
				ArticleID:   c.ArticleID,
				Title:       c.Title,
				FolderID:    c.FolderID,
				BodyExcerpt: c.BodyExcerpt,
			}
			byArticle[c.ArticleID] = res
		} else if res.BodyExcerpt == "" {
			res.BodyExcerpt = c.BodyExcerpt
		}
		res.RRFScore += 1.0 / (k + float64(rank+1))
		if res.KeywordRank == 0 {
			res.KeywordRank = rank + 1
		}
	}
	out := make([]SearchResult, 0, len(byArticle))
	for _, v := range byArticle {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RRFScore > out[j].RRFScore
	})
	return out
}

func rerankerCandidateText(result SearchResult) string {
	var b strings.Builder
	b.WriteString(result.Title)

	limit := min(rerankerCandidateChunks, len(result.MatchedChunks))
	for i := 0; i < limit; i++ {
		b.WriteString("\n---\n")
		b.WriteString(result.MatchedChunks[i].ChunkText)
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
