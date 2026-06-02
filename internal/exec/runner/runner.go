// Package runner is the Go-side dispatcher that runs a Request through
// the Python sandbox service. It mints a short-lived auth token (via
// internal/exec/token), POSTs the request to exec-runner, parses the
// result envelope, writes an audit row, and revokes the token.
//
// This package is caller-agnostic: AI tools, automations, webhooks, and
// any future feature can dispatch through it. The package has no
// knowledge of "tools" or other domain concepts.
//
// Usage:
//
//	res, err := app.execRunner.Execute(ctx, runnerModels.Request{
//	    InvokerUserID:  user.ID,
//	    Code:           script.Code,
//	    Language:       "python",
//	    ConversationID: &conv.ID,
//	    Input:          args,
//	})
//
// Concurrency: Manager is safe to share across goroutines. All state is
// either constant after New() or guarded by sync primitives in the
// circuit breaker.
package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abhinavxd/libredesk/internal/exec/runner/models"
	tokenpkg "github.com/abhinavxd/libredesk/internal/exec/token"
	tokenmodels "github.com/abhinavxd/libredesk/internal/exec/token/models"
	"github.com/abhinavxd/libredesk/internal/secret"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

// Defaults applied when Request leaves a field zero.
const (
	defaultTimeoutSeconds = 60
	defaultMemoryMB       = 256

	// MaxToolDepth is the cap on recursive sdk.tools.call dispatches.
	// Matches the SDK-side check; defense in depth means both sides
	// enforce the cap.
	MaxToolDepth = 3

	// tokenTTLBuffer extends the auth token's lifetime past the script
	// timeout so callbacks made just before the script's timeout can
	// still authenticate. Aligned with the Python service's behavior.
	tokenTTLBuffer = 60 * time.Second

	// httpCallBuffer is the network-layer headroom on top of the script
	// timeout when calling exec-runner. The exec-runner itself enforces
	// the script timeout; we need a slightly longer outer timeout so we
	// see exec-runner's response rather than canceling first.
	httpCallBuffer = 30 * time.Second
)

// Opts is what New takes. Required fields are commented; the others
// have sensible defaults.
type Opts struct {
	// REQUIRED dependencies:
	Token  *tokenpkg.Manager
	DB     *sqlx.DB
	Logger *logf.Logger

	// REQUIRED config (validated by New):

	// SandboxURL is the base URL of the exec-runner service, e.g.
	// "http://localhost:8090". The runner appends "/v1/execute".
	SandboxURL string

	// SandboxToken is the shared service token authorizing this Go
	// process to call exec-runner. Must match what exec-runner is
	// configured with.
	SandboxToken string

	// PlatformBaseURL is what the Python service hands to scripts as
	// their callback target — i.e., where exec-runner reaches Libredesk.
	// Typically the same host:port the Go binary listens on.
	PlatformBaseURL string

	// SystemUserID is the database ID of the System user. Loaded once
	// at boot from the install_settings table or equivalent. The runner
	// uses this when no human invoker is available, and stores it on
	// each token so the auth middleware can resolve system=True calls.
	SystemUserID int

	// Secrets injects operator-managed secrets into the SDK bundle. Nil is
	// accepted for tests and deployments that have not enabled the secrets
	// table yet; production app startup wires this manager explicitly.
	Secrets *secret.Manager

	// OPTIONAL overrides:

	// HTTPClient lets tests inject a custom transport. If nil, a
	// freshly built client with sane production timeouts is used.
	HTTPClient *http.Client

	// CircuitBreakerThreshold is the number of consecutive failures that
	// trip the breaker. Default 5.
	CircuitBreakerThreshold int

	// CircuitBreakerCooldown is how long the breaker stays open after
	// tripping. Default 30s.
	CircuitBreakerCooldown time.Duration
}

// Manager is the runner's public type. Construct with New, share across
// the app.
type Manager struct {
	opts    Opts
	http    *http.Client
	breaker *breaker
	queries *queries
}

// New constructs a Manager and validates required Opts.
func New(opts Opts) (*Manager, error) {
	if opts.Token == nil {
		return nil, errors.New("runner: nil token manager")
	}
	if opts.DB == nil {
		return nil, errors.New("runner: nil db")
	}
	if opts.Logger == nil {
		return nil, errors.New("runner: nil logger")
	}
	if opts.SandboxURL == "" {
		return nil, errors.New("runner: empty sandbox url")
	}
	if opts.SandboxToken == "" {
		return nil, errors.New("runner: empty sandbox token")
	}
	if opts.PlatformBaseURL == "" {
		return nil, errors.New("runner: empty platform base url")
	}
	if opts.SystemUserID == 0 {
		return nil, errors.New("runner: system user id is zero")
	}

	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{
			// Per-call timeout is set on the request context, not the
			// client, because each request has its own script timeout.
			// We do leave a sane idle/connection cap on the transport.
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		}
	}

	threshold := opts.CircuitBreakerThreshold
	if threshold == 0 {
		threshold = 5
	}
	cooldown := opts.CircuitBreakerCooldown
	if cooldown == 0 {
		cooldown = 30 * time.Second
	}

	q, err := initQueries(opts.DB)
	if err != nil {
		return nil, fmt.Errorf("runner: prepare queries: %w", err)
	}

	return &Manager{
		opts:    opts,
		http:    opts.HTTPClient,
		breaker: newBreaker(threshold, cooldown),
		queries: q,
	}, nil
}

// Execute runs a single script and returns its result.
//
// Errors returned here are infrastructure failures (sandbox unavailable,
// HTTP error, depth violation, validation problem). Script-level
// outcomes (failure, error, noop) come back as a non-nil Result with the
// corresponding Kind — *not* as a Go error. Callers should branch on
// res.Kind, not on err alone.
func (m *Manager) Execute(ctx context.Context, req models.Request) (*models.Result, error) {
	if err := m.validateRequest(&req); err != nil {
		return nil, err
	}

	if !m.breaker.allow() {
		return nil, ErrSandboxUnavailable
	}

	// Apply defaults and normalize.
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = defaultTimeoutSeconds
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = defaultMemoryMB
	}
	if req.TraceID == "" {
		req.TraceID = newID("tr")
	}
	if req.InvokerUserID == 0 {
		// No human invoker — system-triggered execution. Run as System.
		req.InvokerUserID = m.opts.SystemUserID
	}

	executionID := newID("exec")
	startedAt := time.Now()

	// Mint the auth token the script will use to call back.
	tokenTTL := time.Duration(req.TimeoutSeconds)*time.Second + tokenTTLBuffer
	tok, err := m.opts.Token.Issue(ctx, tokenmodels.IssueRequest{
		InvokerUserID:  req.InvokerUserID,
		SystemUserID:   m.opts.SystemUserID,
		ExecutionID:    executionID,
		ConversationID: req.ConversationID,
		ToolDepth:      req.ToolDepth,
		ToolName:       req.ToolName,
		TraceID:        req.TraceID,
		TTL:            tokenTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("runner: issue token: %w", err)
	}
	prefixedToken := tokenpkg.PrefixedValue(tok)

	// Always revoke when we're done — even if the call fails, the token
	// shouldn't outlive the dispatch.
	defer func() {
		if err := m.opts.Token.Revoke(context.Background(), prefixedToken); err != nil {
			m.opts.Logger.Warn("runner: revoke token failed",
				"execution_id", executionID, "error", err)
		}
	}()

	secretValues, err := m.loadSecrets(ctx, req)
	if err != nil {
		m.recordAuditError(ctx, executionID, req, startedAt, err)
		m.breaker.recordFailure()
		return nil, err
	}

	// Build the exec-runner payload.
	payload := buildSandboxPayload(req, executionID, prefixedToken, m.opts.PlatformBaseURL, secretValues)

	// Call exec-runner with retry on transient errors.
	resp, err := m.callSandbox(ctx, payload, req.TimeoutSeconds, secretValues)
	if err != nil {
		// Record audit row even on failure so we have an observability
		// trail. Non-fatal — log if audit fails but return the original
		// dispatch error.
		m.recordAuditError(ctx, executionID, req, startedAt, err)
		m.breaker.recordFailure()
		return nil, err
	}

	m.breaker.recordSuccess()

	result := toResult(resp, executionID, startedAt, req.TimeoutSeconds)
	m.recordAudit(ctx, executionID, req, startedAt, result, resp)

	return result, nil
}

// validateRequest checks the structural invariants the runner enforces
// before issuing tokens or doing network I/O.
func (m *Manager) validateRequest(req *models.Request) error {
	if req.Code == "" {
		return ErrEmptyCode
	}
	if req.Language != "python" && req.Language != "javascript" {
		return fmt.Errorf("runner: unsupported language %q: %w",
			req.Language, ErrInvalidRequest)
	}
	if req.ToolDepth < 0 {
		return fmt.Errorf("runner: negative tool depth: %w", ErrInvalidRequest)
	}
	if req.ToolDepth >= MaxToolDepth {
		return ErrDepthLimitExceeded
	}
	if req.TimeoutSeconds < 0 {
		return fmt.Errorf("runner: negative timeout: %w", ErrInvalidRequest)
	}
	if req.MemoryMB < 0 {
		return fmt.Errorf("runner: negative memory: %w", ErrInvalidRequest)
	}
	return nil
}

func (m *Manager) loadSecrets(ctx context.Context, req models.Request) (map[string]string, error) {
	if m.opts.Secrets == nil {
		return nil, nil
	}
	conversationID := 0
	if req.ConversationID != nil {
		conversationID = *req.ConversationID
	}
	values, err := m.opts.Secrets.LoadAll(ctx, secret.AccessContext{
		FunctionName:   req.ToolName,
		ConversationID: conversationID,
		TraceID:        req.TraceID,
	})
	if err != nil {
		return nil, fmt.Errorf("runner: load secrets: %w", err)
	}
	return values, nil
}

// callSandbox does the actual HTTP exchange with retry on retryable
// errors. Returns the parsed sandbox response or a typed error.
func (m *Manager) callSandbox(
	ctx context.Context,
	payload sandboxRequest,
	scriptTimeoutSeconds int,
	secretValues map[string]string,
) (*sandboxResponse, error) {
	url := m.opts.SandboxURL + "/v1/execute"

	// Outer timeout = script timeout + network buffer. The Python
	// service enforces the script timeout itself; this is just so we
	// don't hang forever on a stalled connection.
	httpTimeout := time.Duration(scriptTimeoutSeconds)*time.Second + httpCallBuffer

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("runner: marshal request: %w", err)
	}

	// Retry once on retryable errors. Budget: 2 attempts total.
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, httpTimeout)
		resp, err := m.singleCall(callCtx, url, body, secretValues)
		cancel()

		if err == nil {
			return resp, nil
		}

		lastErr = err
		// Only retry on the explicitly retryable categories. Bad request,
		// not-found, and other 4xxs are not retryable.
		if !isRetryable(err) {
			return nil, err
		}

		// Don't retry if the parent context is done — we'd just fail
		// faster on the second attempt.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		m.opts.Logger.Warn("runner: sandbox call retryable failure",
			"attempt", attempt, "error", err, "url", url)
	}

	return nil, lastErr
}

// singleCall does one HTTP request and parses the response.
func (m *Manager) singleCall(
	ctx context.Context,
	url string,
	body []byte,
	secretValues map[string]string,
) (*sandboxResponse, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, url, bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("runner: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.opts.SandboxToken)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := m.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("runner: http: %w: %w", ErrSandboxTransient, err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("runner: read response: %w: %w",
			ErrSandboxTransient, err)
	}
	switch httpResp.StatusCode {
	case http.StatusOK:
		var sr sandboxResponse
		if err := json.Unmarshal(respBytes, &sr); err != nil {
			return nil, fmt.Errorf("runner: parse 200 response: %w", err)
		}
		return &sr, nil

	case http.StatusServiceUnavailable:
		// Exec-runner reports 503 in two cases: warm pool empty / not
		// ready (transient — retry), or out of capacity (transient).
		// Both retryable per the F6 plan.
		return nil, fmt.Errorf("runner: sandbox 503: %w", ErrSandboxTransient)

	case http.StatusTooManyRequests:
		// Admission gate rejected. Treat as transient.
		return nil, fmt.Errorf("runner: sandbox 429: %w", ErrSandboxTransient)

	case http.StatusUnauthorized:
		// Misconfiguration — service tokens don't match. Don't retry.
		return nil, fmt.Errorf("runner: sandbox auth failed (check EXEC_RUNNER_TOKEN)")

	default:
		if rt, ok := parseRuntimeError(respBytes); ok {
			return nil, rt
		}
		// 4xx (validation errors from us) and unexpected 5xx — don't retry.
		return nil, fmt.Errorf(
			"runner: sandbox returned %d: %s",
			httpResp.StatusCode, truncate(string(respBytes), 500),
		)
	}
}

// recordAudit writes a successful execution row.
//
// On a failure result (result.Kind == KindError) we additionally fold
// result.Stderr into the JSONB Data column under the underscore-prefixed
// _stderr key, so tool authors can pull the traceback out via
// `data->>'_stderr'` without an extra column. The underscore prefix
// signals "operational metadata, distinct from script-emitted data."
func (m *Manager) recordAudit(
	ctx context.Context,
	executionID string,
	req models.Request,
	startedAt time.Time,
	result *models.Result,
	resp *sandboxResponse,
) {
	// Fold stderr into the JSONB blob on failure paths so the audit row
	// carries the debug surface alongside the rest of the result data.
	dataForAudit := result.Data
	if result.Stderr != "" && result.Kind == models.KindError {
		dataMap := map[string]json.RawMessage{}
		if len(result.Data) > 0 && string(result.Data) != "null" {
			if err := json.Unmarshal(result.Data, &dataMap); err != nil {
				dataMap["value"] = result.Data
			}
		}
		stderrJSON, _ := json.Marshal(result.Stderr)
		dataMap["_stderr"] = stderrJSON
		dataForAudit, _ = json.Marshal(dataMap)
	}
	if len(dataForAudit) == 0 || string(dataForAudit) == "null" {
		dataForAudit = json.RawMessage(`{}`)
	}
	if err := m.queries.insertExecution(ctx, executionRow{
		ExecutionID:    executionID,
		ToolName:       nullableString(req.ToolName),
		InvokerUserID:  req.InvokerUserID,
		ConversationID: req.ConversationID,
		ToolDepth:      req.ToolDepth,
		Language:       req.Language,
		StartedAt:      startedAt,
		FinishedAt:     time.Now(),
		DurationMs:     resp.DurationMs,
		Kind:           string(result.Kind),
		ResultCode:     nullableString(result.Code),
		Message:        nullableString(result.Message),
		Data:           dataForAudit,
		WarmPoolHit:    resp.WarmPoolHit,
		TraceID:        nullableString(req.TraceID),
	}); err != nil {
		m.opts.Logger.Error("runner: audit insert failed",
			"execution_id", executionID, "error", err)
	}
}

// RecordInternalAudit writes a function_executions row for an
// internal-capability invocation. Used by the unified dispatcher's
// internal-kind path so audit rows for in-process Go calls share the
// same table + column shape as Python sandbox runs. Language is fixed
// to "internal" so ops can filter the two surfaces apart at query time.
//
// Best-effort: errors are logged, never propagated. Dispatch already
// returned to the caller before this fires.
func (m *Manager) RecordInternalAudit(ctx context.Context, row models.InternalAuditRow) {
	if m == nil || m.queries == nil {
		return
	}
	var convPtr *int
	if row.ConversationID > 0 {
		c := row.ConversationID
		convPtr = &c
	}
	finishedAt := row.StartedAt.Add(time.Duration(row.DurationMs) * time.Millisecond)
	if err := m.queries.insertExecution(ctx, executionRow{
		ExecutionID:    row.ExecutionID,
		ToolName:       nullableString(row.ToolName),
		InvokerUserID:  row.InvokerUserID,
		ConversationID: convPtr,
		ToolDepth:      0,
		Language:       "internal",
		StartedAt:      row.StartedAt,
		FinishedAt:     finishedAt,
		DurationMs:     row.DurationMs,
		Kind:           row.Kind,
		Message:        nullableString(row.Message),
		Data:           row.Data,
		TraceID:        nullableString(row.TraceID),
	}); err != nil {
		m.opts.Logger.Error("runner: internal audit insert failed",
			"execution_id", row.ExecutionID, "error", err)
	}
}

// recordAuditError writes an audit row when the dispatch itself failed
// (no script result to record). Best-effort — never fatal.
func (m *Manager) recordAuditError(
	ctx context.Context,
	executionID string,
	req models.Request,
	startedAt time.Time,
	dispatchErr error,
) {
	if err := m.queries.insertExecution(ctx, executionRow{
		ExecutionID:    executionID,
		ToolName:       nullableString(req.ToolName),
		InvokerUserID:  req.InvokerUserID,
		ConversationID: req.ConversationID,
		ToolDepth:      req.ToolDepth,
		Language:       req.Language,
		StartedAt:      startedAt,
		FinishedAt:     time.Now(),
		Kind:           string(models.KindError),
		ResultCode:     nullableString("DISPATCH_FAILED"),
		Message:        nullableString(truncate(dispatchErr.Error(), 1000)),
		TraceID:        nullableString(req.TraceID),
	}); err != nil {
		m.opts.Logger.Error("runner: audit insert (error path) failed",
			"execution_id", executionID, "error", err)
	}
}

// ───────────── Sandbox wire types ─────────────

// sandboxRequest is the JSON we POST to exec-runner /v1/execute.
// Fields/names exactly match the Python service's ExecuteRequest model.
type sandboxRequest struct {
	Code             string            `json:"code"`
	Language         string            `json:"language"`
	Input            map[string]any    `json:"input,omitempty"`
	ConversationData json.RawMessage   `json:"conversation_data,omitempty"`
	CustomerData     json.RawMessage   `json:"customer_data,omitempty"`
	ContractData     json.RawMessage   `json:"contract_data,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	CallbackToken    string            `json:"callback_token"`
	CallbackBaseURL  string            `json:"callback_base_url"`
	ExecutionID      string            `json:"execution_id"`
	ToolDepth        int               `json:"tool_depth"`
	TimeoutSeconds   int               `json:"timeout_seconds"`
	MemoryMB         int               `json:"memory_mb"`
	Packages         []string          `json:"packages,omitempty"`
	AllowedDomains   []string          `json:"allowed_domains,omitempty"`
	TraceID          string            `json:"trace_id,omitempty"`
}

// sandboxResponse is the JSON we read back from exec-runner /v1/execute.
type sandboxResponse struct {
	ExitCode    int             `json:"exit_code"`
	Stdout      string          `json:"stdout"`
	Stderr      string          `json:"stderr"`
	Result      *resultEnvelope `json:"result"`
	DurationMs  int             `json:"duration_ms"`
	WarmPoolHit bool            `json:"warm_pool_hit"`
}

type resultEnvelope struct {
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data,omitempty"`
	Message   string          `json:"message,omitempty"`
	Code      string          `json:"code,omitempty"`
	Retryable bool            `json:"retryable,omitempty"`
}

type runtimeErrorResponse struct {
	Kind      string `json:"kind"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Traceback string `json:"traceback,omitempty"`
}

// buildSandboxPayload turns a Request into the wire shape exec-runner
// expects.
func buildSandboxPayload(
	req models.Request,
	executionID, callbackToken, platformBase string,
	secretValues map[string]string,
) sandboxRequest {
	return sandboxRequest{
		Code:             req.Code,
		Language:         req.Language,
		Input:            req.Input,
		ConversationData: req.ConversationData,
		CustomerData:     req.CustomerData,
		ContractData:     req.ContractData,
		Secrets:          secretValues,
		CallbackToken:    callbackToken,
		CallbackBaseURL:  platformBase,
		ExecutionID:      executionID,
		ToolDepth:        req.ToolDepth,
		TimeoutSeconds:   req.TimeoutSeconds,
		MemoryMB:         req.MemoryMB,
		Packages:         req.Packages,
		AllowedDomains:   req.AllowedDomains,
		TraceID:          req.TraceID,
	}
}

// toResult converts a sandbox response to the public Result.
//
// Result.Code is normalized to the EXECUTION_* enum so callers don't
// have to switch on raw sandbox codes:
//
//	success path  → CodeExecutionSuccess
//	failure/noop  → existing semantics (Code field carries the
//	                script-emitted code if any; Kind disambiguates)
//	error path    → distinguish by exit code + duration + stderr:
//	                  - duration ≥ 95% of declared timeout → CodeExecutionTimeout
//	                  - stderr starts/contains a Python traceback → CodeExecutionException
//	                  - else → CodeExecutionFailed
//
// stderr is always propagated (possibly empty), capped at MaxStderrBytes.
// requestedTimeoutSeconds is the timeout the caller asked for (passed
// in from Execute); we use it for the 95%-of-timeout heuristic.
func toResult(resp *sandboxResponse, executionID string, startedAt time.Time, requestedTimeoutSeconds int) *models.Result {
	res := &models.Result{
		ExecutionID: executionID,
		DurationMs:  resp.DurationMs,
		WarmPoolHit: resp.WarmPoolHit,
		StartedAt:   startedAt,
		Stderr:      capStderr(resp.Stderr),
	}

	// No result envelope at all → script crashed without calling sdk.*.
	// Apply timeout/exception/failed heuristics.
	if resp.Result == nil {
		res.Kind = models.KindError
		res.Code = classifyFailure(resp, requestedTimeoutSeconds)
		res.Message = failureMessageFor(res.Code, resp)
		return res
	}

	// Sandbox emitted a result envelope — pass through, normalizing Code.
	res.Kind = models.Kind(resp.Result.Kind)
	res.Data = resp.Result.Data
	res.Message = resp.Result.Message
	res.Code = resp.Result.Code
	res.Retryable = resp.Result.Retryable

	// Normalize the success-path code. The sandbox emits whatever the
	// script set on sdk.success(code=...); when no code is provided we
	// stamp the canonical EXECUTION_SUCCESS so callers can rely on it.
	if res.Kind == models.KindSuccess && res.Code == "" {
		res.Code = models.CodeExecutionSuccess
	}

	// Sandbox synthesizes its own NO_RESULT_EMITTED when exit_code=0
	// but no __RESULT__ was found. Treat this as an operational failure
	// (script-author bug — should have called sdk.success/etc.) and
	// classify it the same way as a missing envelope.
	if res.Kind == models.KindError && (res.Code == "NO_RESULT_EMITTED" || res.Code == "NO_RESULT") {
		res.Code = classifyFailure(resp, requestedTimeoutSeconds)
		res.Message = failureMessageFor(res.Code, resp)
	}

	return res
}

// classifyFailure picks one of CodeExecutionTimeout / CodeExecutionException
// / CodeExecutionFailed based on signal-derived hints.
//
// Heuristics, in priority order:
//  1. Duration ≥ 95% of declared timeout → TIMEOUT (the script almost
//     certainly hit the kill signal; standard SIGKILL gives exit_code
//     -1/137 but we don't trust that across platforms).
//  2. Stderr contains "Traceback (most recent call last)" → EXCEPTION.
//  3. Else → FAILED (catch-all: VM error after script started, or
//     non-traceback crash like sys.exit(1) or hard signal).
func classifyFailure(resp *sandboxResponse, requestedTimeoutSeconds int) string {
	if requestedTimeoutSeconds > 0 {
		// Use 95% threshold so we catch slightly-early kills too.
		// (timeout * 1000 ms, then × 0.95).
		threshold := (requestedTimeoutSeconds * 1000 * 95) / 100
		if resp.DurationMs >= threshold {
			return models.CodeExecutionTimeout
		}
	}
	if strings.Contains(resp.Stderr, "Traceback (most recent call last)") {
		return models.CodeExecutionException
	}
	return models.CodeExecutionFailed
}

// failureMessageFor returns a stable, user-facing failure message for
// the EXECUTION_* codes. Tools surface this to authors verbatim so it
// has to read cleanly without context.
func failureMessageFor(code string, resp *sandboxResponse) string {
	switch code {
	case models.CodeExecutionTimeout:
		return fmt.Sprintf("script exceeded its declared timeout (ran %dms before being killed)", resp.DurationMs)
	case models.CodeExecutionException:
		// First non-empty line of stderr is usually the most useful
		// summary (e.g., "ValueError: boom"). Last line of a traceback
		// is the exception message.
		return summarizeTraceback(resp.Stderr)
	default:
		return "script terminated without emitting a result"
	}
}

// summarizeTraceback extracts the most useful one-liner from a Python
// traceback — typically the last non-empty line, which is the exception
// type + message (e.g., "ValueError: boom").
func summarizeTraceback(stderr string) string {
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" {
			return "script raised: " + s
		}
	}
	return "script raised an unhandled exception"
}

// capStderr enforces MaxStderrBytes with a clear truncation marker so
// readers know the original was bigger.
func capStderr(s string) string {
	if len(s) <= models.MaxStderrBytes {
		return s
	}
	// Keep the LAST MaxStderrBytes — for Python tracebacks, the tail
	// (where the actual exception message lives) is more useful than
	// the head (where stack frames start).
	tail := s[len(s)-models.MaxStderrBytes:]
	return fmt.Sprintf("...[truncated; original %d bytes]\n%s", len(s), tail)
}

// ───────────── Errors ─────────────

var (
	// ErrInvalidRequest covers all caller-side request validation
	// failures. Wraps a more specific error message.
	ErrInvalidRequest = errors.New("runner: invalid request")

	// ErrEmptyCode is returned when Request.Code is empty.
	ErrEmptyCode = fmt.Errorf("runner: empty code: %w", ErrInvalidRequest)

	// ErrDepthLimitExceeded is returned when a request would push
	// recursion past MaxToolDepth.
	ErrDepthLimitExceeded = errors.New("runner: tool depth limit exceeded")

	// ErrSandboxUnavailable is returned when the circuit breaker is
	// open. Callers should surface this as a retryable platform error
	// to the script.
	ErrSandboxUnavailable = errors.New("runner: sandbox unavailable")

	// ErrSandboxTransient wraps a retryable sandbox failure. The retry
	// loop in callSandbox uses this to decide whether to retry.
	ErrSandboxTransient = errors.New("runner: sandbox transient error")
)

// RuntimeError is returned when exec-runner itself failed after accepting a
// request, before a script-level result could be produced.
type RuntimeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Traceback string `json:"traceback,omitempty"`
}

func (e *RuntimeError) Error() string {
	if e == nil {
		return "runner: runtime error"
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("runner: runtime error: %s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return "runner: runtime error: " + e.Message
	}
	if e.Code != "" {
		return "runner: runtime error: " + e.Code
	}
	return "runner: runtime error"
}

func parseRuntimeError(body []byte) (*RuntimeError, bool) {
	var out runtimeErrorResponse
	if err := json.Unmarshal(body, &out); err != nil || out.Kind != "runtime_error" {
		return nil, false
	}
	return &RuntimeError{
		Code:      out.Code,
		Message:   out.Message,
		Traceback: out.Traceback,
	}, true
}

// isRetryable returns true if err is or wraps a retryable category.
func isRetryable(err error) bool {
	return errors.Is(err, ErrSandboxTransient)
}

// ───────────── Helpers ─────────────

// newID returns a short identifier with the given prefix, e.g.
// newID("exec") → "exec_a1b2c3d4". 8 bytes of entropy is plenty for
// short-lived IDs we deduplicate by random-only collision avoidance.
func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Should never happen on a sane system. Fall back to time-based
		// uniqueness so we don't crash a critical path.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ───────────── Circuit breaker ─────────────

// breaker is a tiny in-memory circuit breaker. After `threshold`
// consecutive failures it stays open for `cooldown`, then enters a
// half-open state where the next request decides whether to close or
// reopen.
//
// Intentionally simpler than sony/gobreaker — we don't need its
// stat tracking or per-request budget. Just open/closed/half-open.
type breaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration

	failures int
	openedAt time.Time // zero when closed
}

func newBreaker(threshold int, cooldown time.Duration) *breaker {
	return &breaker{threshold: threshold, cooldown: cooldown}
}

// allow reports whether a call should proceed.
func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.openedAt.IsZero() {
		return true
	}
	// Open — allow only after cooldown has elapsed (half-open probe).
	return time.Since(b.openedAt) >= b.cooldown
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedAt = time.Time{}
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold {
		// Refresh openedAt on every threshold-or-above failure. This
		// matters for the half-open transition: if a probe call (the
		// first allowed call after cooldown) fails, we need to restart
		// the cooldown rather than stay stuck in a permanent half-open
		// state where every call is allowed.
		b.openedAt = time.Now()
	}
}
