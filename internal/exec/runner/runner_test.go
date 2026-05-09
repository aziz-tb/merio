package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abhinavxd/libredesk/internal/exec/runner/models"
	tokenpkg "github.com/abhinavxd/libredesk/internal/exec/token"
	"github.com/alicebob/miniredis/v2"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// ─── Test fixtures ───

// testEnv is the bundle of infrastructure each test needs — sandbox HTTP
// stub, redis (via miniredis), DB (via SQLite memory), and a Manager
// pointing at all of it. Returns a teardown for cleanup.
type testEnv struct {
	sandbox   *httptest.Server
	calls     *atomic.Int32
	lastBody  *atomicString
	mgr       *Manager
	tokenMgr  *tokenpkg.Manager
	redis     *redis.Client
	miniredis *miniredis.Miniredis
}

// atomicString is a small concurrency-safe string holder; httptest
// handlers may run in different goroutines than the test.
type atomicString struct {
	mu sync.Mutex
	v  string
}

func (a *atomicString) set(s string) { a.mu.Lock(); a.v = s; a.mu.Unlock() }
func (a *atomicString) get() string  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }

// sandboxOK returns an exec-runner stub that responds 200 with a success
// envelope. The test can override the response shape via the optional
// override.
func sandboxOK(t *testing.T, calls *atomic.Int32, lastBody *atomicString, override func(w http.ResponseWriter, r *http.Request) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		lastBody.set(string(body))

		if override != nil && override(w, r) {
			return
		}

		// Default: 200 success
		resp := sandboxResponse{
			ExitCode: 0,
			Stdout:   `__RESULT__ {"kind": "success", "data": {"hello": "world"}}`,
			Result: &resultEnvelope{
				Kind: "success",
				Data: map[string]any{"hello": "world"},
			},
			DurationMs:  42,
			WarmPoolHit: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// setupSchema creates the script_executions table in an in-memory SQLite.
// The schema is intentionally a subset of the Postgres production schema —
// just enough columns to satisfy the INSERT in queries.sql.
func setupSchema(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const schema = `
CREATE TABLE script_executions (
    execution_id     TEXT PRIMARY KEY,
    tool_name        TEXT,
    invoker_user_id  INTEGER NOT NULL,
    conversation_id  INTEGER,
    tool_depth       INTEGER NOT NULL DEFAULT 0,
    language         TEXT NOT NULL,
    started_at       DATETIME NOT NULL,
    finished_at      DATETIME,
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    kind             TEXT NOT NULL,
    result_code      TEXT,
    message          TEXT,
    data             TEXT,                -- jsonb in prod; text here
    warm_pool_hit    BOOLEAN NOT NULL DEFAULT 0,
    trace_id         TEXT
);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func newTestEnv(t *testing.T, sandboxOverride func(w http.ResponseWriter, r *http.Request) bool) *testEnv {
	t.Helper()

	mr := miniredis.RunT(t)
	rd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rd.Close() })

	lo := logf.New(logf.Opts{})

	tokMgr, err := tokenpkg.New(tokenpkg.Opts{Redis: rd, Logger: &lo})
	if err != nil {
		t.Fatalf("token.New: %v", err)
	}

	calls := &atomic.Int32{}
	lastBody := &atomicString{}
	sb := sandboxOK(t, calls, lastBody, sandboxOverride)
	t.Cleanup(sb.Close)

	db := setupSchema(t)

	mgr, err := New(Opts{
		Token:                   tokMgr,
		DB:                      db,
		Logger:                  &lo,
		SandboxURL:              sb.URL,
		SandboxToken:            "sandbox-secret",
		PlatformBaseURL:         "http://localhost:8080",
		SystemUserID:            1,
		CircuitBreakerThreshold: 3,
		CircuitBreakerCooldown:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return &testEnv{
		sandbox:   sb,
		calls:     calls,
		lastBody:  lastBody,
		mgr:       mgr,
		tokenMgr:  tokMgr,
		redis:     rd,
		miniredis: mr,
	}
}

func validRequest() models.Request {
	return models.Request{
		InvokerUserID: 17,
		Code:          `import sdk; sdk.success(data={"ok": True})`,
		Language:      "python",
		Input:         map[string]any{"foo": "bar"},
	}
}

// ─── New (validation) ───

func TestNew_RejectsMissingFields(t *testing.T) {
	mr := miniredis.RunT(t)
	rd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rd.Close()
	lo := logf.New(logf.Opts{})
	tok, _ := tokenpkg.New(tokenpkg.Opts{Redis: rd, Logger: &lo})
	db := setupSchema(t)

	cases := []struct {
		name string
		opts Opts
	}{
		{"nil token mgr", Opts{DB: db, Logger: &lo, SandboxURL: "x", SandboxToken: "x", PlatformBaseURL: "x", SystemUserID: 1}},
		{"nil db", Opts{Token: tok, Logger: &lo, SandboxURL: "x", SandboxToken: "x", PlatformBaseURL: "x", SystemUserID: 1}},
		{"nil logger", Opts{Token: tok, DB: db, SandboxURL: "x", SandboxToken: "x", PlatformBaseURL: "x", SystemUserID: 1}},
		{"empty sandbox url", Opts{Token: tok, DB: db, Logger: &lo, SandboxToken: "x", PlatformBaseURL: "x", SystemUserID: 1}},
		{"empty sandbox token", Opts{Token: tok, DB: db, Logger: &lo, SandboxURL: "x", PlatformBaseURL: "x", SystemUserID: 1}},
		{"empty platform url", Opts{Token: tok, DB: db, Logger: &lo, SandboxURL: "x", SandboxToken: "x", SystemUserID: 1}},
		{"zero system user id", Opts{Token: tok, DB: db, Logger: &lo, SandboxURL: "x", SandboxToken: "x", PlatformBaseURL: "x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// ─── Execute happy path ───

func TestExecute_Success(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	res, err := env.mgr.Execute(ctx, validRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsSuccess() {
		t.Fatalf("Kind = %q, want success", res.Kind)
	}
	if res.Data["hello"] != "world" {
		t.Fatalf("Data missing expected key: %#v", res.Data)
	}
	if res.ExecutionID == "" {
		t.Fatal("ExecutionID empty")
	}
	if !res.WarmPoolHit {
		t.Fatal("expected WarmPoolHit=true")
	}

	// One sandbox call only — happy path doesn't retry.
	if got := env.calls.Load(); got != 1 {
		t.Fatalf("sandbox calls = %d, want 1", got)
	}
}

func TestExecute_PassesPayloadShape(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	convID := 42
	req.ConversationID = &convID
	req.ToolName = "fetch_invoices"
	req.Packages = []string{"requests==2.32.3"}
	req.AllowedDomains = []string{"api.example.com"}

	if _, err := env.mgr.Execute(ctx, req); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var sent sandboxRequest
	if err := json.Unmarshal([]byte(env.lastBody.get()), &sent); err != nil {
		t.Fatalf("decode last body: %v", err)
	}

	if sent.Code != req.Code {
		t.Errorf("Code mismatch")
	}
	if sent.Language != "python" {
		t.Errorf("Language = %q", sent.Language)
	}
	if !strings.HasPrefix(sent.CallbackToken, "sx_") {
		t.Errorf("CallbackToken should be sx_-prefixed: %q", sent.CallbackToken)
	}
	if sent.CallbackBaseURL != "http://localhost:8080" {
		t.Errorf("CallbackBaseURL = %q", sent.CallbackBaseURL)
	}
	if sent.MemoryMB != defaultMemoryMB {
		t.Errorf("MemoryMB = %d, want %d", sent.MemoryMB, defaultMemoryMB)
	}
	if sent.TimeoutSeconds != defaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds = %d, want %d", sent.TimeoutSeconds, defaultTimeoutSeconds)
	}
	if len(sent.Packages) != 1 || sent.Packages[0] != "requests==2.32.3" {
		t.Errorf("Packages = %v", sent.Packages)
	}
	if sent.TraceID == "" {
		t.Error("TraceID should be auto-generated")
	}
}

func TestExecute_TokenIsRevokedAfterDispatch(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	if _, err := env.mgr.Execute(ctx, validRequest()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// After dispatch, the token's redis key should be gone. Inspect
	// miniredis's keys directly.
	keys := env.miniredis.Keys()
	for _, k := range keys {
		if strings.HasPrefix(k, "exec:token:") {
			t.Errorf("token key %q still present after Execute", k)
		}
	}
}

func TestExecute_AuditRowWritten(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	res, err := env.mgr.Execute(ctx, validRequest())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var count int
	if err := env.mgr.opts.DB.Get(&count,
		"SELECT COUNT(*) FROM script_executions WHERE execution_id = ?",
		res.ExecutionID); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit row count = %d, want 1", count)
	}

	var kind string
	if err := env.mgr.opts.DB.Get(&kind,
		"SELECT kind FROM script_executions WHERE execution_id = ?",
		res.ExecutionID); err != nil {
		t.Fatalf("get kind: %v", err)
	}
	if kind != "success" {
		t.Errorf("kind = %q, want success", kind)
	}
}

// ─── Execute validation ───

func TestExecute_RejectsEmptyCode(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	req.Code = ""

	_, err := env.mgr.Execute(ctx, req)
	if !errors.Is(err, ErrEmptyCode) {
		t.Fatalf("err = %v, want ErrEmptyCode", err)
	}
}

func TestExecute_RejectsUnknownLanguage(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	req.Language = "ruby"

	_, err := env.mgr.Execute(ctx, req)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want wrapped ErrInvalidRequest", err)
	}
}

func TestExecute_RejectsExcessiveDepth(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	req.ToolDepth = MaxToolDepth // strictly equal to cap is rejected

	_, err := env.mgr.Execute(ctx, req)
	if !errors.Is(err, ErrDepthLimitExceeded) {
		t.Fatalf("err = %v, want ErrDepthLimitExceeded", err)
	}
	if env.calls.Load() != 0 {
		t.Error("sandbox should not be called on rejected depth")
	}
}

func TestExecute_NegativeDepth(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	req.ToolDepth = -1

	_, err := env.mgr.Execute(ctx, req)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestExecute_ZeroInvoker_FallsBackToSystem(t *testing.T) {
	env := newTestEnv(t, nil)
	ctx := context.Background()

	req := validRequest()
	req.InvokerUserID = 0 // simulate system-triggered execution

	res, err := env.mgr.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Kind != models.KindSuccess {
		t.Fatalf("Kind = %q, want success", res.Kind)
	}
	// Verify the audit row records the system user as invoker.
	var invoker int
	if err := env.mgr.opts.DB.Get(&invoker,
		"SELECT invoker_user_id FROM script_executions WHERE execution_id = ?",
		res.ExecutionID); err != nil {
		t.Fatalf("get invoker: %v", err)
	}
	if invoker != 1 {
		t.Errorf("invoker_user_id = %d, want 1 (system)", invoker)
	}
}

// ─── Sandbox failure paths ───

func TestExecute_Sandbox500_ReturnsError(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		http.Error(w, "boom", http.StatusInternalServerError)
		return true
	})
	ctx := context.Background()

	_, err := env.mgr.Execute(ctx, validRequest())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecute_Sandbox503_RetriesOnce(t *testing.T) {
	var calls atomic.Int32
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return true
		}
		// Second attempt: succeed.
		return false
	})
	ctx := context.Background()

	res, err := env.mgr.Execute(ctx, validRequest())
	if err != nil {
		t.Fatalf("Execute should succeed on retry: %v", err)
	}
	if res.Kind != models.KindSuccess {
		t.Fatalf("Kind = %q", res.Kind)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 sandbox calls, got %d", got)
	}
}

func TestExecute_Sandbox503Twice_ReturnsTransient(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		http.Error(w, "warming up", http.StatusServiceUnavailable)
		return true
	})
	ctx := context.Background()

	_, err := env.mgr.Execute(ctx, validRequest())
	if !errors.Is(err, ErrSandboxTransient) {
		t.Fatalf("err = %v, want wrapping ErrSandboxTransient", err)
	}
}

func TestExecute_Sandbox401_DoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		calls.Add(1)
		http.Error(w, "bad auth", http.StatusUnauthorized)
		return true
	})
	ctx := context.Background()

	_, err := env.mgr.Execute(ctx, validRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call (no retry on 401), got %d", got)
	}
}

// ─── Circuit breaker ───

func TestExecute_CircuitBreakerTripsAfterThreshold(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return true
	})
	ctx := context.Background()

	// Threshold is 3 (set in newTestEnv). Drive 3 dispatches that all fail.
	for i := 0; i < 3; i++ {
		_, _ = env.mgr.Execute(ctx, validRequest())
	}

	// The next call should be rejected without hitting the sandbox.
	callsBefore := env.calls.Load()
	_, err := env.mgr.Execute(ctx, validRequest())
	if !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("err = %v, want ErrSandboxUnavailable", err)
	}
	if env.calls.Load() != callsBefore {
		t.Error("expected no sandbox call when breaker is open")
	}
}

func TestExecute_CircuitBreakerClosesAfterCooldown(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) bool {
		if failing.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return true
		}
		return false // fall through to default success
	})
	ctx := context.Background()

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		_, _ = env.mgr.Execute(ctx, validRequest())
	}
	// Confirm it's open.
	if _, err := env.mgr.Execute(ctx, validRequest()); !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("breaker should be open: %v", err)
	}

	// Wait past the cooldown (100ms in newTestEnv).
	time.Sleep(150 * time.Millisecond)
	failing.Store(false)

	// Half-open probe should succeed and close the breaker.
	res, err := env.mgr.Execute(ctx, validRequest())
	if err != nil {
		t.Fatalf("post-cooldown call should succeed: %v", err)
	}
	if !res.IsSuccess() {
		t.Fatalf("Kind = %q", res.Kind)
	}
}

// ─── breaker unit tests (independent of HTTP) ───

func TestBreaker_OpensAtThreshold(t *testing.T) {
	b := newBreaker(3, 100*time.Millisecond)
	if !b.allow() {
		t.Fatal("fresh breaker should allow")
	}
	b.recordFailure()
	b.recordFailure()
	if !b.allow() {
		t.Fatal("breaker should still allow at 2 failures")
	}
	b.recordFailure() // hits threshold
	if b.allow() {
		t.Fatal("breaker should be open at threshold")
	}
}

func TestBreaker_ResetsOnSuccess(t *testing.T) {
	b := newBreaker(3, 100*time.Millisecond)
	b.recordFailure()
	b.recordFailure()
	b.recordSuccess()
	b.recordFailure()
	b.recordFailure()
	if !b.allow() {
		t.Fatal("breaker should still allow — success reset the counter")
	}
}

func TestBreaker_HalfOpensAfterCooldown(t *testing.T) {
	b := newBreaker(2, 50*time.Millisecond)
	b.recordFailure()
	b.recordFailure()
	if b.allow() {
		t.Fatal("expected open")
	}
	time.Sleep(80 * time.Millisecond)
	if !b.allow() {
		t.Fatal("expected half-open after cooldown")
	}
}

// TestBreaker_FailedProbeRestartsCooldown guards against a regression in
// the half-open behavior. After cooldown, the breaker allows one probe.
// If that probe fails, the breaker must re-arm — i.e., further calls
// should be denied until the next cooldown elapses, not allowed
// indefinitely.
func TestBreaker_FailedProbeRestartsCooldown(t *testing.T) {
	b := newBreaker(2, 50*time.Millisecond)
	b.recordFailure()
	b.recordFailure() // breaker opens

	time.Sleep(80 * time.Millisecond)
	if !b.allow() {
		t.Fatal("expected half-open allow after cooldown")
	}

	// Probe fails.
	b.recordFailure()

	// Next call should be denied — the cooldown should have been
	// restarted by the probe's failure.
	if b.allow() {
		t.Fatal("breaker should be re-opened after failed probe; bug regression")
	}

	// Wait for the new cooldown.
	time.Sleep(80 * time.Millisecond)
	if !b.allow() {
		t.Fatal("expected half-open allow after second cooldown")
	}
}
