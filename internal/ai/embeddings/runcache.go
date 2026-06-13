package embeddings

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
)

// SearchResultCache caches kb_search results for the lifetime of one
// agent run; not safe for cross-run reuse. Goroutine-safe for parallel
// tool dispatch, with duplicate in-flight misses coalesced via singleflight.
type SearchResultCache struct {
	mu      sync.Mutex
	entries map[string][]SearchResult
	sf      singleflight.Group
}

// NewSearchResultCache builds an empty per-run cache.
func NewSearchResultCache() *SearchResultCache {
	return &SearchResultCache{entries: make(map[string][]SearchResult)}
}

// Get returns cached results for the given key, or false on miss.
func (c *SearchResultCache) Get(key string) ([]SearchResult, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return cloneSearchResults(r), true
}

// Set stores results under the given key; last-write-wins on duplicates.
func (c *SearchResultCache) Set(key string, results []SearchResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cloneSearchResults(results)
}

// GetOrLoad returns cached results if available; otherwise runs load and
// stores the resulting slice. Concurrent misses with the same key are
// coalesced into a single underlying load.
//
// Nil-safe: a nil cache invokes load directly without caching.
// Errors are not cached.
func (c *SearchResultCache) GetOrLoad(
	key string,
	load func() ([]SearchResult, error),
) ([]SearchResult, error) {
	if c == nil {
		return load()
	}

	if cached, ok := c.Get(key); ok {
		return cached, nil
	}

	v, err, _ := c.sf.Do(key, func() (any, error) {
		if cached, ok := c.Get(key); ok {
			return cached, nil
		}
		results, err := load()
		if err != nil {
			return nil, err
		}
		c.Set(key, results)
		return cloneSearchResults(results), nil
	})
	if err != nil {
		return nil, err
	}

	results, _ := v.([]SearchResult)
	return cloneSearchResults(results), nil
}

// RunCacheKey builds the canonical cache key for a SearchInput from the
// fields that change results — Query (trimmed, case-preserved), ContentType,
// FolderIDs (sorted), Locale, TopK. The *Candidates tuning knobs are
// excluded: the model can't observe them, so they must not split the cache.
func RunCacheKey(in SearchInput) string {
	ids := append([]int64(nil), in.FolderIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}

	var b strings.Builder
	b.WriteString(strings.TrimSpace(in.Query))
	b.WriteByte('|')
	b.WriteString(in.ContentType)
	b.WriteByte('|')
	b.WriteString(strings.Join(idStrs, ","))
	b.WriteByte('|')
	b.WriteString(in.Locale)
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(in.TopK))
	return b.String()
}

func cloneSearchResults(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]SearchResult, len(results))
	copy(out, results)
	return out
}
