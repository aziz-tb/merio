package secret

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resolverLookup struct {
	mu    sync.Mutex
	val   string
	ok    bool
	err   error
	calls int
	last  AccessContext
}

func (l *resolverLookup) Get(_ context.Context, _ string, access AccessContext) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	l.last = access
	return l.val, l.ok, l.err
}

func (l *resolverLookup) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func (l *resolverLookup) lastAccess() AccessContext {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

func TestResolverSecretWinsOverEnvAndCaches(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-value")
	lookup := &resolverLookup{val: "secret-value", ok: true}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := newResolverWithTTL(lookup, time.Minute, func() time.Time { return now })

	require.Equal(t, "secret-value", r.Resolve(context.Background(), "OPENAI_API_KEY"))
	require.Equal(t, "secret-value", r.Resolve(context.Background(), "OPENAI_API_KEY"))
	require.Equal(t, 1, lookup.count())
	require.Equal(t, "secret.Resolver", lookup.lastAccess().FunctionName)
}

func TestResolverCacheExpiryRefetches(t *testing.T) {
	lookup := &resolverLookup{val: "first", ok: true}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := newResolverWithTTL(lookup, time.Minute, func() time.Time { return now })

	require.Equal(t, "first", r.Resolve(context.Background(), "AZURE_API_KEY"))

	lookup.mu.Lock()
	lookup.val = "second"
	lookup.mu.Unlock()
	now = now.Add(2 * time.Minute)

	require.Equal(t, "second", r.Resolve(context.Background(), "AZURE_API_KEY"))
	require.Equal(t, 2, lookup.count())
}

func TestResolverFallsBackToEnv(t *testing.T) {
	t.Setenv("VOYAGE_ENDPOINT", "https://env.example/embeddings")

	for _, tc := range []struct {
		name   string
		lookup *resolverLookup
	}{
		{name: "missing secret", lookup: &resolverLookup{ok: false}},
		{name: "empty secret", lookup: &resolverLookup{val: "", ok: true}},
		{name: "lookup error", lookup: &resolverLookup{err: errors.New("db down")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(tc.lookup)
			require.Equal(t, "https://env.example/embeddings", r.Resolve(context.Background(), "VOYAGE_ENDPOINT"))
		})
	}
}

func TestResolverConcurrentResolve(t *testing.T) {
	lookup := &resolverLookup{val: "secret-value", ok: true}
	r := NewResolver(lookup)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.Equal(t, "secret-value", r.Resolve(context.Background(), "ANTHROPIC_API_KEY"))
		}()
	}
	wg.Wait()
}

func TestResolverNilFallsBackToEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-value")
	require.Equal(t, "env-value", (*Resolver)(nil).Resolve(context.Background(), "OPENAI_API_KEY"))
	require.Equal(t, "", (*Resolver)(nil).Resolve(context.Background(), "MISSING_KEY"))
	require.Equal(t, "", os.Getenv("MISSING_KEY"))
}
