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
// Cache key format: "embed:<model>:<sha256(query)>". Different models
// have different vector spaces; co-mingling would return the wrong
// vector after a model upgrade. Including the model in the key
// invalidates the cache automatically on model swap.
//
// Cache miss is silent on read error — Redis being down should never
// make hybrid search fail. The provider call is the load-bearing
// dependency; the cache is a latency optimization.
type QueryEmbeddingCache struct {
	rdb *redis.Client
	ttl time.Duration
	lo  *logf.Logger
}

// NewQueryEmbeddingCache builds a cache. Pass a non-zero ttl
// (typical: 1h). Pass a logger so cache failures surface in the same
// log stream.
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
