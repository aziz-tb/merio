// Package models defines the shapes the runner's caller fills in (Request)
// and the runner returns (Result). Callers include the AI subsystem
// (registered tool dispatch), automations, webhooks, and any future
// in-process feature that wants to run sandboxed code.
//
// These types are intentionally caller-agnostic. The runner doesn't know
// or care what a "tool" is — it only knows how to ship a chunk of code
// into the Python sandbox, mint a callback token, and parse the result.
package models

import (
	"encoding/json"
	"time"
)

// Request describes a single script execution. The runner fills in the
// missing operational fields (callback URL, packages defaults, timeout
// defaults, etc.) and ships the request to the Python sandbox service.
type Request struct {
	// Identity (REQUIRED):
	//
	// InvokerUserID is the user the script will act as by default. For
	// human-triggered executions this is the agent who initiated the
	// action; for API-key-triggered executions it's the API key's owner;
	// for system-triggered executions (inbound email, scheduled jobs)
	// the runner substitutes the System user automatically — callers
	// can leave this 0 in that case.
	InvokerUserID int

	// Code & language (REQUIRED):
	Code     string // the script source
	Language string // "python" or "javascript"

	// Optional sandbox settings. Zero values use sensible defaults
	// chosen by the runner (see runner.defaultMemoryMB, defaultTimeout).
	Packages       []string
	AllowedDomains []string
	TimeoutSeconds int
	MemoryMB       int

	// Inputs piped to the SDK inside the sandbox.
	//
	// Input becomes sdk.input — a regular Python dict the script reads
	// to get its arguments.
	Input map[string]any

	// ConversationData (optional) becomes sdk.conversation_data. When set,
	// it must be the platform's full Conversation JSON shape (the runner
	// doesn't transform it). Pass the raw bytes from the existing
	// conversation manager's serialization to keep the shape consistent.
	ConversationData json.RawMessage

	// CustomerData (optional) becomes sdk.customer_data. Reserved for
	// future per-org executor output; v1 leaves this nil and the SDK
	// exposes None.
	CustomerData json.RawMessage

	// ContractData (optional) becomes sdk.contract_data. Reserved like
	// CustomerData; v1 leaves this nil.
	ContractData json.RawMessage

	// Recursion + tracing.
	//
	// ConversationID is optional. When set, the runner stores it on the
	// auth token so handlers can read it for default-scoping.
	ConversationID *int

	// ToolName is the registered tool name when this execution is a tool
	// invocation. Empty for ad-hoc executions (e.g., automation script
	// actions). Used in audit log attribution and the auth token.
	ToolName string

	// ToolDepth tracks recursive sdk.tools.call dispatches. 0 for
	// top-level dispatches. The runner rejects requests where
	// ToolDepth >= MaxToolDepth before dispatching.
	ToolDepth int

	// TraceID correlates with sandbox-side logs. If empty the runner
	// generates one.
	TraceID string
}

// Kind enumerates the four possible script outcomes. These mirror the
// SDK's sdk.success/sdk.failure/sdk.error/sdk.noop calls. Anything else
// (uncaught exception, no result emitted at all) ends up as KindError
// with a code identifying the underlying cause (see Code* below).
type Kind string

const (
	KindSuccess Kind = "success"
	KindFailure Kind = "failure"
	KindError   Kind = "error"
	KindNoop    Kind = "noop"
)

// Result.Code values. Stable enum used by callers (cmd/ai_tools.go) to
// pick HTTP status codes:
//
//	CodeExecutionSuccess   → script ran and called sdk.success / sdk.noop / sdk.failure
//	CodeExecutionTimeout   → script ran past its declared timeout and got killed (408)
//	CodeExecutionException → script raised an unhandled exception; stderr has the traceback (422)
//	CodeExecutionFailed    → script crashed for some other reason, or exited cleanly without
//	                         calling sdk.* (500-class — internal/operational)
//
// Distinction is made Go-side by inspecting exit_code, duration vs
// declared timeout, and whether stderr looks like a Python traceback.
// The Python sandbox doesn't currently surface these codes itself; if
// it ever does, the codes here become the canonical names and the Go
// runner just passes them through.
const (
	CodeExecutionSuccess   = "EXECUTION_SUCCESS"
	CodeExecutionTimeout   = "EXECUTION_TIMEOUT"
	CodeExecutionException = "EXECUTION_EXCEPTION"
	CodeExecutionFailed    = "EXECUTION_FAILED"
)

// MaxStderrBytes caps Stderr bytes we propagate / persist. Long Python
// tracebacks aren't useful past this; the Result envelope and the
// tool_executions.data._stderr field both honor this cap and add a
// trailing marker on truncation.
const MaxStderrBytes = 8 * 1024

// Result is what the runner returns to the caller after the script has
// completed (or failed). It mirrors the structure of the sandbox's
// ExecuteResponse but with named Go-side fields.
type Result struct {
	Kind      Kind           `json:"kind"`
	Data      map[string]any `json:"data,omitempty"`
	Message   string         `json:"message,omitempty"`
	Code      string         `json:"code,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`

	// Stderr captures the script's stderr stream, regardless of outcome.
	// Always present on returned Result (may be empty). Capped at
	// MaxStderrBytes; if truncated, ends with a marker line.
	//
	// Tool authors get the traceback for Python exceptions here; for
	// successful runs it usually carries logging output. This field is
	// the primary debug surface when EXECUTION_EXCEPTION fires.
	Stderr string `json:"stderr,omitempty"`

	// Operational telemetry — exposed mostly for the AI pipeline to log,
	// not for handlers.
	ExecutionID string    `json:"execution_id"`
	DurationMs  int       `json:"duration_ms"`
	WarmPoolHit bool      `json:"warm_pool_hit"`
	StartedAt   time.Time `json:"started_at"`
}

// IsSuccess is a small convenience for callers that only care whether the
// script terminated cleanly.
func (r *Result) IsSuccess() bool {
	return r != nil && r.Kind == KindSuccess
}

// InternalAuditRow is the input shape for Manager.RecordInternalAudit.
// Mirrors the columns of the tool_executions table that apply to
// in-process Go capability invocations (no warm-pool, no result-code
// from the sandbox, language is hardcoded to "internal" inside the
// runner). Kept in this package so the dispatcher (which lives outside
// the runner package) can construct rows without a circular import.
type InternalAuditRow struct {
	ExecutionID    string
	ToolName       string
	InvokerUserID  int
	ConversationID int
	StartedAt      time.Time
	DurationMs     int
	Kind           string          // "success" | "error"
	Message        string          // populated on Kind="error"
	Data           json.RawMessage // populated on Kind="success" with the marshalled capability output
	TraceID        string
}
