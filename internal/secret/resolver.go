package secret

import (
	"context"
	"os"
	"sync"
	"time"
)

const defaultResolverTTL = 5 * time.Minute

// SecretLookup is the minimum database lookup surface Resolver needs. Manager
// satisfies this interface via Get.
type SecretLookup interface {
	Get(ctx context.Context, name string, access AccessContext) (string, bool, error)
}

// Resolver returns provider API keys and endpoint URLs, preferring values from
// the secrets table over OS env vars. Results are cached in memory for five
// minutes so key and endpoint rotations take effect without an app restart.
type Resolver struct {
	lookup SecretLookup
	ttl    time.Duration
	now    func() time.Time

	mu    sync.RWMutex
	cache map[string]cachedSecret
}

type cachedSecret struct {
	value     string
	expiresAt time.Time
}

// NewResolver builds a Resolver with the production five-minute TTL.
func NewResolver(lookup SecretLookup) *Resolver {
	return newResolverWithTTL(lookup, defaultResolverTTL, time.Now)
}

func newResolverWithTTL(lookup SecretLookup, ttl time.Duration, now func() time.Time) *Resolver {
	if ttl <= 0 {
		ttl = defaultResolverTTL
	}
	if now == nil {
		now = time.Now
	}
	return &Resolver{
		lookup: lookup,
		ttl:    ttl,
		now:    now,
		cache:  make(map[string]cachedSecret),
	}
}

// Resolve returns the value for name. Secrets table values win over env vars;
// empty or missing secrets fall through to env. Empty output means the caller
// should treat the config value as unset.
func (r *Resolver) Resolve(ctx context.Context, name string) string {
	if r == nil {
		return os.Getenv(name)
	}
	now := r.now()

	r.mu.RLock()
	if cs, ok := r.cache[name]; ok && now.Before(cs.expiresAt) {
		r.mu.RUnlock()
		return cs.value
	}
	r.mu.RUnlock()

	val := r.fetch(ctx, name)

	r.mu.Lock()
	r.cache[name] = cachedSecret{value: val, expiresAt: r.now().Add(r.ttl)}
	r.mu.Unlock()

	return val
}

func (r *Resolver) fetch(ctx context.Context, name string) string {
	if r.lookup != nil {
		access := AccessContext{FunctionName: "secret.Resolver"}
		if v, ok, err := r.lookup.Get(ctx, name, access); err == nil && ok && v != "" {
			return v
		}
	}
	return os.Getenv(name)
}
