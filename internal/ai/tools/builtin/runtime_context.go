// Package builtin defines the in-process capability registry used by the
// AI agent runtime. Capabilities are registered at startup via init()
// blocks under .../builtin/capabilities/*; the runtime resolves them
// through Registry, builds a ToolRuntimeContext per run, and invokes Execute.
//
// Named "builtin" not "internal" because Go's internal-directory rule would
// block the sibling internal/ai/runner/ subtree (where the runtime lives)
// from importing the registry.
package builtin

import (
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/conversation"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/knowledge"
	"github.com/aziz-tb/merio/internal/user"
)

// KBScope is the runtime representation of the agent's kb_scope JSONB
// column. Mirrors agents/models.KBScope but lives here to avoid a
// runtime → agents/models import cycle.
type KBScope struct {
	FolderIDs  []int
	ArticleIDs []int
}

// ToolRuntimeContext carries per-run state for tool capabilities. One
// instance is built at the start of an agent run and reused across ALL
// tool dispatches within that run.
//
// GOROUTINE SAFETY INVARIANT (permanent): parallel tool dispatch within a
// turn means multiple capability goroutines may access this struct
// concurrently, so ALL FIELDS MUST BE GOROUTINE-SAFE. When adding fields:
// protect mutable maps/slices with a mutex or a safe wrapper (e.g.
// SearchResultCache embeds sync.Mutex), use atomics for counters, and treat
// read-only-after-construction fields as safe. Verify new shared state with
// `go test -race ./internal/ai/runner/...`. The invariant holds as long as
// dispatch is parallel.
//
// Zero values are valid sentinels; capabilities that need a value should
// error rather than misread 0 as a real id:
//   - ConversationID == 0: not in a conversation context (test endpoint, ad-hoc admin call).
//   - AgentID == 0: outside agent context (rewrite, Copilot, etc.).
//   - ResolutionAttemptID == 0: not in the inbound pipeline yet.
//   - VerificationLevel == 0: default-public bucket.
//
// Capabilities run through the unified dispatcher, which validates
// parameters and applies verification gating before Execute; they may
// assume those checks ran but should still guard against bad input.
type ToolRuntimeContext struct {
	// Per-run identity.
	ConversationID      int
	AgentID             int
	ResolutionAttemptID int
	TraceID             string
	VerificationLevel   int

	// KBScope is the agent's whitelist over knowledge-base content, decoded
	// from ai_agents.kb_scope once per Run(). kb_search enforces intersection
	// semantics against it:
	//   - FolderIDs caps the LLM-passed folder_ids (narrow, never widen); an
	//     out-of-scope folder surfaces as a soft error, not silent filtering.
	//   - ArticleIDs are additive, whitelisted regardless of folder scope.
	// Zero-value (both nil) means no scope set, so the capability falls
	// through to unscoped behavior (test harness, direct-invoke flows).
	KBScope KBScope

	// KBSearchCache caches kb_search results within a single run. Nil-safe:
	// Get/Set/GetOrLoad on a nil pointer fall through to a real search.
	KBSearchCache *embeddings.SearchResultCache

	// Logf is the call-scoped logger; logging through it inherits trace_id
	// and other structured fields.
	Logf *logf.Logger

	// Manager handles the runtime injects so capabilities can read existing
	// data without the HTTP API. Only managers used by a registered
	// capability live here; add more when the first consumer ships.
	Knowledge    *knowledge.Manager
	Conversation *conversation.Manager
	User         *user.Manager
	Customer     *customer.Manager
	Contract     *contract.Manager

	// Future hooks (don't build until needed):
	//   DB           *sqlx.DB                 — raw query escape hatch
	//   PythonRunner *execrunner.Manager      — for Python-wrapping caps
	//   RequestCache *requestcache.Cache      — generic request-scoped dedup
}
