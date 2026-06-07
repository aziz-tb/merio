// Package builtin defines the in-process capability registry used by the
// AI agent runtime. Capabilities are first-class Go functions registered
// at startup via init() blocks under .../builtin/capabilities/*; the
// agent runtime resolves them through Registry, builds a
// ToolRuntimeContext per run, and invokes Execute.
//
// Why "builtin" not "internal": Go's `internal` directory rule restricts
// imports to the subtree rooted at the parent. Using `internal` here
// would mean `internal/ai/runner/` (a sibling subtree) could not import
// the registry, which is exactly where the agent runtime lives.
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
// column. Mirrors agents/models.KBScope but lives in the builtin package
// to avoid a runtime → agents/models import (the builtin package is
// imported by agents/models indirectly via the runner).
type KBScope struct {
	FolderIDs  []int
	ArticleIDs []int
}

// ToolRuntimeContext carries per-run state available to tool
// capabilities during dispatch. A single ToolRuntimeContext is
// constructed at the start of an agent run and reused across ALL tool
// dispatches within that run.
//
// =====================================================================
// GOROUTINE SAFETY INVARIANT (permanent)
// =====================================================================
//
// Parallel tool dispatch within a single turn means multiple capability
// goroutines may access this struct concurrently. ALL FIELDS ON
// ToolRuntimeContext MUST BE GOROUTINE-SAFE.
//
// When adding new fields, follow these patterns:
//
//   - Mutable maps/slices: protect with a mutex, or use a goroutine-safe
//     wrapper type (for example SearchResultCache embeds sync.Mutex).
//   - Singleflight groups: golang.org/x/sync/singleflight.Group is safe
//     by design.
//   - Read-only after construction: safe with no protection.
//   - Counters: use sync/atomic primitives.
//   - Plain primitives that mutate during dispatch: protect with a
//     mutex or atomic operations.
//
// Race-detector tests are encouraged for new shared state. Run with
// `go test -race ./internal/ai/runner/...` to catch violations.
//
// This invariant holds until dispatch is changed back to sequential
// (which would be a regression). Plan accordingly.
// =====================================================================
//
// The capability decides which fields it needs; many capabilities won't
// touch most of them. Zero values are valid sentinel values:
//   - ConversationID == 0 means "not invoked in a conversation context"
//     (e.g., test endpoint, ad-hoc admin call). Capabilities that need
//     it should error explicitly rather than misread 0 as "id 0".
//   - AgentID == 0 means "outside agent context" (rewrite, Copilot, etc.).
//   - ResolutionAttemptID == 0 means "not in the inbound pipeline yet"
//     (foundation isn't here; placeholder for F3).
//   - VerificationLevel == 0 is the default-public bucket.
//
// Capabilities are invoked through the unified dispatcher (chunk 10) which
// validates parameters and applies verification gating before calling
// Execute. They MAY assume those checks have already happened, but should
// still be robust to bad input — defensive checks are cheap.
type ToolRuntimeContext struct {
	// Per-run identity.
	ConversationID      int
	AgentID             int
	ResolutionAttemptID int
	TraceID             string
	VerificationLevel   int

	// KBScope is the agent's whitelist over knowledge-base content. The
	// runner decodes ai_agents.kb_scope JSONB once per Run() and stuffs
	// it here before every dispatch. kb_search reads this to enforce
	// intersection semantics:
	//   - FolderIDs caps the LLM-passed folder_ids (LLM can narrow, never
	//     widen). An out-of-scope folder is surfaced as a soft error the
	//     LLM can course-correct from, not silent filtering.
	//   - ArticleIDs are additive — whitelisted regardless of folder scope.
	// Zero-value (both nil) means "no scope set"; outside the runner path
	// (test harness, future direct-invoke flows) the capability falls
	// through to the unscoped behavior.
	KBScope KBScope

	// KBSearchCache caches kb_search results within this single run.
	// Nil-safe: capabilities may call Get/Set/GetOrLoad on a nil cache
	// pointer; misses simply fall through to a real search.
	KBSearchCache *embeddings.SearchResultCache

	// Logger scoped to this call. Capabilities log via rt.Logf to
	// inherit the trace_id and other structured fields.
	Logf *logf.Logger

	// Manager handles the runtime injects so capabilities can read
	// Merio's existing data without going through the HTTP API.
	// Only the managers actually used by registered capabilities live
	// here; add more (DB, PythonRunner, RequestCache) when the first
	// capability that needs them ships.
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
