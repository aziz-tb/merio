package token

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abhinavxd/libredesk/internal/exec/token/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// newTestManager spins up a miniredis instance and returns a Manager
// wired to it, plus the underlying miniredis for assertions and TTL
// fast-forwarding.
func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)

	rd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rd.Close() })

	lo := logf.New(logf.Opts{})
	mgr, err := New(Opts{Redis: rd, Logger: &lo})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return mgr, mr
}

func validIssueRequest() models.IssueRequest {
	convID := 99
	return models.IssueRequest{
		InvokerUserID:  17,
		SystemUserID:   1,
		ExecutionID:    "exec_abc",
		ConversationID: &convID,
		ToolDepth:      0,
		ToolName:       "fetch_invoices",
		TraceID:        "tr_xyz",
		TTL:            5 * time.Minute,
	}
}

// ─── New ───

func TestNew_RejectsNilRedis(t *testing.T) {
	lo := logf.New(logf.Opts{})
	if _, err := New(Opts{Redis: nil, Logger: &lo}); err == nil {
		t.Fatal("expected error for nil redis")
	}
}

func TestNew_RejectsNilLogger(t *testing.T) {
	mr := miniredis.RunT(t)
	rd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rd.Close()
	if _, err := New(Opts{Redis: rd, Logger: nil}); err == nil {
		t.Fatal("expected error for nil logger")
	}
}

// ─── Issue ───

func TestIssue_StoresTokenInRedis(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, validIssueRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok.Value == "" {
		t.Fatal("Issue returned empty token value")
	}

	// Verify the key exists in Redis with the expected key prefix.
	key := redisKey(tok.Value)
	if _, err := mr.Get(key); err != nil {
		t.Fatalf("expected key %q in redis: %v", key, err)
	}
}

func TestIssue_GeneratesUniqueValues(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		tok, err := mgr.Issue(ctx, validIssueRequest())
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if _, ok := seen[tok.Value]; ok {
			t.Fatalf("duplicate token value: %q", tok.Value)
		}
		seen[tok.Value] = struct{}{}
	}
}

func TestIssue_ValidatesRequiredFields(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	cases := []struct {
		name string
		mod  func(*models.IssueRequest)
	}{
		{"missing invoker user id", func(r *models.IssueRequest) { r.InvokerUserID = 0 }},
		{"missing system user id", func(r *models.IssueRequest) { r.SystemUserID = 0 }},
		{"missing execution id", func(r *models.IssueRequest) { r.ExecutionID = "" }},
		{"zero ttl", func(r *models.IssueRequest) { r.TTL = 0 }},
		{"negative ttl", func(r *models.IssueRequest) { r.TTL = -1 * time.Second }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validIssueRequest()
			tc.mod(&req)
			if _, err := mgr.Issue(ctx, req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestIssue_AppliesTTL(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	req := validIssueRequest()
	req.TTL = 10 * time.Second
	tok, err := mgr.Issue(ctx, req)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// miniredis exposes the TTL via TTL() — should be > 0 and <= TTL.
	ttl := mr.TTL(redisKey(tok.Value))
	if ttl <= 0 || ttl > 10*time.Second {
		t.Fatalf("unexpected TTL %v, want > 0 && <= 10s", ttl)
	}
}

// ─── Resolve ───

func TestResolve_InvokerMode_ReturnsInvokerUser(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, validIssueRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	res, err := mgr.Resolve(ctx, PrefixedValue(tok), models.ActorInvoker)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.UserID != 17 {
		t.Fatalf("UserID = %d, want 17 (invoker)", res.UserID)
	}
	if res.Mode != models.ActorInvoker {
		t.Fatalf("Mode = %q, want invoker", res.Mode)
	}
	if res.Token.ExecutionID != "exec_abc" {
		t.Fatalf("ExecutionID = %q, want exec_abc", res.Token.ExecutionID)
	}
}

func TestResolve_SystemMode_ReturnsSystemUser(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, validIssueRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	res, err := mgr.Resolve(ctx, PrefixedValue(tok), models.ActorSystem)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.UserID != 1 {
		t.Fatalf("UserID = %d, want 1 (system)", res.UserID)
	}
}

func TestResolve_MissingPrefix_ReturnsNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.Resolve(ctx, "abc123", models.ActorInvoker)
	if err == nil {
		t.Fatal("expected error")
	}
	// Should wrap ErrTokenNotFound (caller treats all "not usable" cases
	// uniformly so we don't leak why).
	if !contains(err.Error(), "not found") {
		t.Fatalf("error %q should mention 'not found'", err.Error())
	}
}

func TestResolve_UnknownToken_ReturnsNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.Resolve(ctx, "sx_doesnotexist", models.ActorInvoker)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "not found") {
		t.Fatalf("error %q should mention 'not found'", err.Error())
	}
}

func TestResolve_ExpiredByRedisTTL_ReturnsNotFound(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, validIssueRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Fast-forward miniredis past the TTL.
	mr.FastForward(10 * time.Minute)

	_, err = mgr.Resolve(ctx, PrefixedValue(tok), models.ActorInvoker)
	if err == nil {
		t.Fatal("expected error after expiry")
	}
}

func TestResolve_StoredExpiryPassed_ReturnsNotFound(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	// Manually plant a token with stored expiry in the past, but a long
	// Redis TTL — defensive case the Resolve check guards against.
	stale := `{"value":"stale","invoker_user_id":1,"system_user_id":1,"execution_id":"e","tool_depth":0,"expires_at":"2020-01-01T00:00:00Z"}`
	if err := mr.Set(redisKey("stale"), stale); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mr.SetTTL(redisKey("stale"), 1*time.Hour)

	_, err := mgr.Resolve(ctx, "sx_stale", models.ActorInvoker)
	if err == nil {
		t.Fatal("expected error for stale token")
	}
}

func TestResolve_InvalidActorMode(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	tok, _ := mgr.Issue(ctx, validIssueRequest())

	_, err := mgr.Resolve(ctx, PrefixedValue(tok), models.ActorMode("bogus"))
	if err == nil {
		t.Fatal("expected error for unknown actor mode")
	}
}

func TestResolve_CorruptedJSON_ReturnsNotFound(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	if err := mr.Set(redisKey("corrupt"), "not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := mgr.Resolve(ctx, "sx_corrupt", models.ActorInvoker)
	if err == nil {
		t.Fatal("expected error for corrupted token")
	}
}

// ─── Revoke ───

func TestRevoke_RemovesToken(t *testing.T) {
	mgr, mr := newTestManager(t)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, validIssueRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	prefixed := PrefixedValue(tok)

	if err := mgr.Revoke(ctx, prefixed); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, err := mr.Get(redisKey(tok.Value)); err == nil {
		t.Fatal("expected key removed from redis")
	}
}

func TestRevoke_MissingToken_NotError(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	if err := mgr.Revoke(ctx, "sx_doesnotexist"); err != nil {
		t.Fatalf("Revoke missing should be idempotent, got %v", err)
	}
}

// ─── PrefixedValue ───

func TestPrefixedValue(t *testing.T) {
	tok := &models.Token{Value: "abc"}
	got := PrefixedValue(tok)
	if got != "sx_abc" {
		t.Fatalf("PrefixedValue = %q, want sx_abc", got)
	}
}

// ─── Helpers ───

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
