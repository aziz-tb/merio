// Package capabilities holds the platform's built-in InternalCapability
// implementations. Each capability lives in its own file with an init()
// block that registers it against the package-level builtin.Default()
// registry. cmd/main.go imports this package (blank import) so the init
// blocks fire at startup.
package capabilities

import (
	"context"
	"fmt"

	"github.com/aziz-tb/merio/internal/ai/embeddings"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
)

func init() {
	builtin.RegisterInternal(defaultKBSearch)
}

// defaultKBSearch is the package-level singleton registered at init time.
// Its searcher is nil until SetKBSearcher runs (from cmd/init.go); Execute
// returns a clear error in that window rather than empty results.
var defaultKBSearch = &kbSearchCapability{}

// kbSearchCapability — hybrid (semantic + keyword + reranker) KB search.
// Four KB-scope semantics:
//
//  1. Out-of-scope folder → soft error to LLM ("course-correct" pattern).
//  2. Article-scope branch when KBScope.ArticleIDs is non-empty.
//  3. Empty agent scope (both arrays non-nil but empty) → no access.
//  4. Nil agent scope (test harness / direct invoke) → unscoped.
//
// agent kb_scope is the authoritative gate; the is_ai_groundable column is
// legacy and deliberately ignored.
type kbSearchCapability struct {
	searcher *embeddings.Searcher
}

// SetKBSearcher injects the hybrid-search engine. Called once from
// cmd/init.go after the embedding subsystem boots, before any Execute.
func SetKBSearcher(s *embeddings.Searcher) {
	defaultKBSearch.searcher = s
}

func (k *kbSearchCapability) ID() string   { return "kb_search" }
func (k *kbSearchCapability) Name() string { return "Search the knowledge base" }

func (k *kbSearchCapability) Description() string {
	return "Search the knowledge base for relevant articles using hybrid retrieval " +
		"(semantic + keyword + cross-encoder reranking). Returns up to `limit` articles ranked by relevance, " +
		"each with title, body excerpt, matched chunk text(s) showing why the article was selected, " +
		"folder context, and ranking scores.\n\n" +
		"Each result includes a `reranker_score` between 0 and 1:\n" +
		"- >= 0.6: strong match, the article likely answers the query\n" +
		"- 0.3-0.6: moderate relevance, may contain partial info\n" +
		"- < 0.3: weak match, the article is unlikely to help\n\n" +
		"If your first search returns strong matches (>= 0.6), additional searches " +
		"with reformulated queries rarely surface better results — the same " +
		"articles will rank highly under different phrasings. Reformulate only " +
		"when no result is strong AND you can think of a meaningfully different " +
		"angle on the question.\n\n" +
		"Use this when the customer asks a factual question — pricing, policies, how-to instructions — " +
		"that the company's documentation is likely to answer. " +
		"Phrase the query naturally; the search understands paraphrasing AND exact-term matching " +
		"(e.g., contract IDs, product codes, technical terms).\n\n" +
		"Always run kb_search BEFORE drafting a response so quotes are grounded in the article rather " +
		"than the model's general knowledge. Cite the matched chunk in your response when relevant."
}

func (k *kbSearchCapability) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Natural-language search query. Both keywords and paraphrasing work.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum results to return (1-20; default 5).",
				"minimum":     1,
				"maximum":     20,
			},
			"folder_ids": map[string]any{
				"type":        "array",
				"description": "Optional KB folder ids to restrict the search to. When omitted, searches all folders the agent has scope for.",
				"items":       map[string]any{"type": "integer"},
			},
		},
		"required": []string{"query"},
	}
}

func (k *kbSearchCapability) OutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"article_id":     map[string]any{"type": "integer"},
						"folder_id":      map[string]any{"type": "integer"},
						"title":          map[string]any{"type": "string"},
						"body_excerpt":   map[string]any{"type": "string"},
						"matched_chunks": map[string]any{"type": "array"},
						"rrf_score":      map[string]any{"type": "number"},
						"reranker_score": map[string]any{"type": "number"},
					},
				},
			},
			"truncated": map[string]any{"type": "boolean"},
		},
	}
}

func (k *kbSearchCapability) Category() string { return "read" }

// Execute runs the four-way scope dispatch and delegates to the hybrid
// searcher.
func (k *kbSearchCapability) Execute(ctx context.Context, params map[string]any, rt *builtin.ToolRuntimeContext) (any, error) {
	if k.searcher == nil {
		return nil, fmt.Errorf("kb_search: searcher not configured (RAG subsystem not initialized?)")
	}

	queryRaw, ok := params["query"].(string)
	if !ok || queryRaw == "" {
		return nil, fmt.Errorf("kb_search: query is required and must be a non-empty string")
	}

	limit := 5
	if v, ok := paramInt(params, "limit"); ok {
		if v < 1 || v > 20 {
			return nil, fmt.Errorf("kb_search: limit must be between 1 and 20 (got %d)", v)
		}
		limit = v
	}

	// folder_ids — LLM-supplied subset to intersect with the agent's
	// scope. Arrives as []any per JSON; coerce.
	folderIDs, err := paramIntSlice(params, "folder_ids")
	if err != nil {
		return nil, fmt.Errorf("kb_search: %w", err)
	}

	hasAgentScope := rt != nil && (rt.KBScope.FolderIDs != nil || rt.KBScope.ArticleIDs != nil)

	if hasAgentScope {
		// 1) Out-of-scope folder → soft error ("course-correct").
		allowed := make(map[int]struct{}, len(rt.KBScope.FolderIDs))
		for _, fid := range rt.KBScope.FolderIDs {
			allowed[fid] = struct{}{}
		}
		for _, fid := range folderIDs {
			if _, ok := allowed[fid]; !ok {
				return nil, fmt.Errorf(
					"kb_search: folder_id=%d is outside this agent's KB scope; agent-scoped folder_ids=%v",
					fid, rt.KBScope.FolderIDs,
				)
			}
		}
		// Empty LLM folder_ids = "all folders the agent has access to".
		if len(folderIDs) == 0 {
			folderIDs = append([]int{}, rt.KBScope.FolderIDs...)
		}
		// 3) Empty agent scope (both arrays empty) → no access.
		if len(folderIDs) == 0 && len(rt.KBScope.ArticleIDs) == 0 {
			return map[string]any{"results": []any{}, "truncated": false}, nil
		}
	}

	// Article scope is applied as a post-filter after the hybrid search
	// returns (see the article-scope branch below).
	folderIDs64 := toInt64Slice(folderIDs)

	locale := "de" // KB is German energy-domain.
	searchInput := embeddings.SearchInput{
		Query:       queryRaw,
		ContentType: "kb_chunk",
		FolderIDs:   folderIDs64,
		Locale:      locale,
		TopK:        limit,
	}

	var cache *embeddings.SearchResultCache
	if rt != nil {
		cache = rt.KBSearchCache
	}
	key := embeddings.RunCacheKey(searchInput)
	results, err := cache.GetOrLoad(key, func() ([]embeddings.SearchResult, error) {
		return k.searcher.Search(ctx, searchInput)
	})
	if err != nil {
		return nil, fmt.Errorf("kb_search: %w", err)
	}

	// 2) Article-scope branch — merge in whitelisted articles that matched
	// the query but fell outside the folder scope, up to the limit.
	if hasAgentScope && len(rt.KBScope.ArticleIDs) > 0 && len(results) < limit {
		seen := make(map[int64]struct{}, len(results))
		for _, r := range results {
			seen[r.ArticleID] = struct{}{}
		}
		extra, err := k.searchArticleScope(ctx, queryRaw, rt, rt.KBScope.ArticleIDs, locale, limit-len(results), seen)
		if err == nil {
			results = append(results, extra...)
		}
	}

	return map[string]any{
		"results":   results,
		"truncated": len(results) >= limit,
	}, nil
}

// searchArticleScope reruns the hybrid search without a folder filter and
// post-filters the results to the agent's whitelisted article ids.
func (k *kbSearchCapability) searchArticleScope(
	ctx context.Context,
	query string,
	rt *builtin.ToolRuntimeContext,
	articleIDs []int,
	locale string,
	limit int,
	seen map[int64]struct{},
) ([]embeddings.SearchResult, error) {
	if limit <= 0 {
		return nil, nil
	}
	// Allowlist as map.
	allow := make(map[int64]struct{}, len(articleIDs))
	for _, aid := range articleIDs {
		allow[int64(aid)] = struct{}{}
	}
	searchInput := embeddings.SearchInput{
		Query:       query,
		ContentType: "kb_chunk",
		Locale:      locale,
		TopK:        limit * 4, // overshoot; will trim after filter
	}

	var cache *embeddings.SearchResultCache
	if rt != nil {
		cache = rt.KBSearchCache
	}
	key := embeddings.RunCacheKey(searchInput)
	res, err := cache.GetOrLoad(key, func() ([]embeddings.SearchResult, error) {
		return k.searcher.Search(ctx, searchInput)
	})
	if err != nil {
		return nil, err
	}
	out := make([]embeddings.SearchResult, 0, limit)
	for _, r := range res {
		if _, dup := seen[r.ArticleID]; dup {
			continue
		}
		if _, ok := allow[r.ArticleID]; !ok {
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// toInt64Slice coerces []int → []int64 for SQL pq.Int64Array.
func toInt64Slice(in []int) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}
