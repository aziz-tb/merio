package embeddings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// QueryEmbeddingCache caches query embeddings in Redis with a TTL.
//
// Key is "embed:<model>:<sha256(query)>"; the model segment invalidates
// the cache automatically on a model swap (vector spaces differ).
// Read errors are treated as a miss so Redis downtime never fails search.
type QueryEmbeddingCache struct {
	rdb *redis.Client
	ttl time.Duration
	lo  *logf.Logger
}

// NewQueryEmbeddingCache builds a cache; a non-positive ttl defaults to 1h.
func NewQueryEmbeddingCache(rdb *redis.Client, ttl time.Duration, lo *logf.Logger) (*QueryEmbeddingCache, error) {
	if rdb == nil {
		return nil, errors.New("embeddings.QueryEmbeddingCache: rdb is required")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	if lo == nil {
		return nil, errors.New("embeddings.QueryEmbeddingCache: Lo is required")
	}
	return &QueryEmbeddingCache{rdb: rdb, ttl: ttl, lo: lo}, nil
}

// Get returns the cached vector for (query, model) or (nil, false) on
// miss / error. Errors are logged and treated as miss.
func (c *QueryEmbeddingCache) Get(ctx context.Context, query, model string) ([]float32, bool) {
	val, err := c.rdb.Get(ctx, c.key(query, model)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		c.lo.Warn("embeddings cache get failed", "error", err)
		return nil, false
	}
	var vec []float32
	if err := json.Unmarshal(val, &vec); err != nil {
		c.lo.Warn("embeddings cache decode failed", "error", err)
		return nil, false
	}
	return vec, true
}

// Set writes the (query, model)→vector entry with the configured TTL.
// Errors are logged and otherwise swallowed.
func (c *QueryEmbeddingCache) Set(ctx context.Context, query, model string, vec []float32) {
	raw, err := json.Marshal(vec)
	if err != nil {
		c.lo.Warn("embeddings cache encode failed", "error", err)
		return
	}
	if err := c.rdb.Set(ctx, c.key(query, model), raw, c.ttl).Err(); err != nil {
		c.lo.Warn("embeddings cache set failed", "error", err)
	}
}

func (c *QueryEmbeddingCache) key(query, model string) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("embed:%s:%s", model, hex.EncodeToString(h[:]))
}
