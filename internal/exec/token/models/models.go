// Package models holds the shared types for short-lived script execution
// tokens. The token is what a script presents in its `Authorization` header
// when it calls back into Merio; resolving it returns the actor context
// (which user the script is acting as).
package models

import "time"

// ActorMode controls which user identity the script's HTTP callback acts
// as for a given request.
//
//   - ActorInvoker: act as the user that triggered the execution (a human
//     agent, an API-key owner, or the System user when there was no human
//     trigger).
//   - ActorSystem: act as the System user, regardless of the invoker. This
//     is the override the SDK requests via `system=True`.
type ActorMode string

const (
	ActorInvoker ActorMode = "invoker"
	ActorSystem  ActorMode = "system"
)

// Token is the metadata stored in Redis under the script's auth key.
//
// The Token is created by the dispatcher when starting an execution and
// deleted (or expired) when the execution finishes. Its lifetime tracks
// the script's lifetime plus a small buffer so callbacks made just before
// the script's timeout can still authenticate.
type Token struct {
	// Random portion of the token. The full Authorization value the script
	// sends is "sx_<Token>", and the secret half of the Token-auth tuple
	// is the same value (see authenticateUser for why).
	Value string `json:"value"`

	// User the script acts as by default. Set when an execution is
	// triggered by a human agent or by an API-key call; falls back to
	// SystemUserID when there's no human trigger.
	InvokerUserID int `json:"invoker_user_id"`

	// User the script acts as when system=True is requested per call.
	// Loaded from config (the System user, role=Admin) at dispatcher init.
	SystemUserID int `json:"system_user_id"`

	// Per-execution context. ConversationID is optional (some triggers
	// have no conversation, e.g., scheduled jobs).
	ExecutionID    string `json:"execution_id"`
	ConversationID *int   `json:"conversation_id,omitempty"`

	// Recursion depth for sdk.tools.call. The dispatcher reads this when a
	// child execution is requested and rejects when it would exceed the
	// global cap (see runner.MaxToolDepth).
	ToolDepth int `json:"tool_depth"`

	// Optional name of the tool whose code is running. Used in audit log
	// attribution. Empty for ad-hoc executions (e.g., automation script
	// actions that aren't registered tools).
	ToolName string `json:"tool_name,omitempty"`

	// Correlates with sandbox-side and pipeline-side log entries.
	TraceID string `json:"trace_id,omitempty"`

	// Wall-clock expiry. Redis TTL also enforces this, but storing it
	// explicitly makes Resolve() defensively reject tokens that somehow
	// outlived their Redis TTL (unlikely, but cheap to check).
	ExpiresAt time.Time `json:"expires_at"`
}

// IssueRequest is what the dispatcher passes to token.Manager.Issue.
type IssueRequest struct {
	InvokerUserID  int
	SystemUserID   int
	ExecutionID    string
	ConversationID *int
	ToolDepth      int
	ToolName       string
	TraceID        string
	TTL            time.Duration
}

// ResolveResult is what authenticateUser gets back when validating a
// script's Authorization header. The middleware uses ActorMode (from the
// X-Script-System header) to pick which user ID to load and authenticate.
type ResolveResult struct {
	Token  *Token
	UserID int       // resolved per ActorMode (invoker or system)
	Mode   ActorMode // which mode was applied
}
