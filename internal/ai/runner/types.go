// Package runner is the LLM↔tool loop that turns an inbound customer
// message + an agent definition into a customer-facing draft. It's the
// validation milestone for the F7+F5 foundation: every prior chunk
// (provider abstraction, preset registry, capability registry, resolver,
// translator, dispatcher) is plumbing the runner consumes.
//
// The runner is a primitive — it doesn't know about the inbound pipeline
// or the composer; those wrap it later. What it does:
//
//  1. Loads an agent (ai_agents row) by id.
//  2. Resolves agent.tools to ResolvedTools (internal + custom).
//  3. Translates ResolvedTools to provider-agnostic llm.ToolDef.
//  4. Builds the initial messages (system prompt + caller-supplied context + inbound).
//  5. Loops with the LLM provider, dispatching tool_use blocks through the
//     unified dispatcher, accumulating cache markers, until the model
//     calls produce_draft or a terminal failure condition fires.
//  6. Returns RunResponse with the final draft, status, full per-turn
//     trace, and aggregate metrics.
//
// All terminal conditions map to one of 8 RunStatus values; downstream
// consumers (composer, GATE, pipeline) switch on these to decide what
// to do with the draft. Conflating them at this layer is a bug —
// "partial" and "incomplete" carry different operational signals.
package runner

import (
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// RunStatus enumerates every terminal outcome of a Run() call. Each
// value carries downstream meaning that pipeline GATE / composer / ops
// dashboards rely on; do NOT conflate them when adding a new path.
type RunStatus string

const (
	// StatusComplete — assistant committed a final draft via the
	// mandatory produce_draft terminal tool. Pipeline GATE can consider
	// sending.
	StatusComplete RunStatus = "complete"

	// StatusAwaitingApproval — the run hit a tool whose slug is in the
	// approval_required list and stopped BEFORE executing it (run-ends-at-
	// write, Slice 4). RunResponse.ProposedWrite carries the proposed tool +
	// inputs; a human approves/edits/rejects and a resuming run executes it.
	// Terminal for this run; not a failure.
	StatusAwaitingApproval RunStatus = "awaiting_approval"

	// StatusPartial — reserved for future flows that intentionally
	// commit partial drafts. The Batch 4 produce_draft contract does not
	// currently emit this status from the agent runner.
	StatusPartial RunStatus = "partial"

	// StatusRefused — reserved for future flows that explicitly commit a
	// refusal as a draft. The Batch 4 produce_draft contract routes
	// uncommitted refusals to StatusIncomplete.
	StatusRefused RunStatus = "refused"

	// StatusBudgetExceeded — reserved for future flows. The Batch 4
	// produce_draft contract routes budget exits without a committed
	// draft to StatusIncomplete.
	StatusBudgetExceeded RunStatus = "budget_exceeded"

	// StatusDeadlineExceeded — reserved for future flows. The Batch 4
	// produce_draft contract routes deadline exits without a committed
	// draft to StatusIncomplete.
	StatusDeadlineExceeded RunStatus = "deadline_exceeded"

	// StatusMaxIterations — reserved for future flows. The Batch 4
	// produce_draft contract routes max-iteration exits without a
	// committed draft to StatusIncomplete.
	StatusMaxIterations RunStatus = "max_iterations"

	// StatusIncomplete — the run ended without a successful
	// produce_draft commit, or otherwise violated the platform contract
	// for producing a customer-facing draft.
	StatusIncomplete RunStatus = "incomplete"

	// StatusFailed — unrecoverable error (provider terminal failure
	// after retries, agent disabled, ctx cancelled, malformed config).
	// The accompanying error from Run() carries the diagnostic.
	StatusFailed RunStatus = "failed"
)

// ConversationMessage is one prior message on the current ticket, fed
// into the runner so the agent can ground its reply in the conversation
// so far. v1 carries only sender + text — no timestamps, no author IDs,
// no message UUIDs. Sender is either "customer" or "agent".
//
// The runner formats this slice into a system message before the LLM
// call (see buildInitialMessages); the format spec is shared across
// every caller — operator-driven Composer runs today, pipeline-driven
// runs when F3/F8 ship — so the agent's contract with the LLM stays
// uniform regardless of who invoked it.
type ConversationMessage struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

// RunRequest is the input to Runner.Run. Every field has a sane default
// when zero (Run applies them); tests / the test endpoint may set knobs
// to exercise budget paths.
type RunRequest struct {
	// AgentID is the ai_agents row id. Required.
	AgentID int

	// InboundMessage is the customer-facing text the agent reasons
	// over. Required.
	InboundMessage string

	// ConversationHistory is the prior messages on the current ticket,
	// oldest-first, excluding the latest customer message (which lives
	// in InboundMessage). Nil/empty when there's no prior turn — the
	// first message of a brand-new conversation, the agent-test panel
	// invoking with synthetic input, etc. Capped upstream (the Composer
	// tab caps at 20 messages for v1; longer summarization is deferred).
	//
	// Distinct from the get_conversation_history capability, which
	// surfaces PRIOR conversations from the same customer. This field
	// is the CURRENT ticket's history, formatted into a system message
	// the LLM sees before the user turn.
	ConversationHistory []ConversationMessage

	// Context is caller-supplied pre-prepared context (customer data,
	// contract data, history). v1 placeholder: JSON-stringified into
	// a system message after the agent's main prompt. Pipeline stage 8
	// will refine into structured per-agent views.
	Context map[string]any

	// ConversationID flows through to the ToolRuntimeContext so
	// capabilities reading conversation scope get default-scoping.
	// Zero means "no conversation in scope" — the test endpoint
	// passes whatever the admin provides.
	ConversationID int

	// InboxID stamps every ai_usage row this run produces. The HTTP
	// handler loads the conversation and resolves its inbox; the
	// AgentTestPanel and any non-conversation caller passes the System
	// inbox id (see internal/migrations/v2_7_7.go::SystemInboxUUID).
	// Required for persistence — the runner refuses to call
	// Usage.Record when zero, but the run itself still proceeds.
	InboxID int

	// VerificationLevel gates agent execution upstream and controls PII
	// surfacing inside capabilities like get_customer_data.
	VerificationLevel int

	// MaxIterations caps the agent loop. Defaults to agent.max_iterations
	// (which itself defaults to 5 per the F9 schema).
	MaxIterations int

	// TotalTokenBudget is a hard ceiling on input+output tokens
	// across all turns. Defaults to defaultTotalTokenBudget (50000).
	// Checked at the TOP of each iteration — pre-flight, not
	// post-flight, so we never start a turn we can't afford.
	TotalTokenBudget int

	// Deadline is the wall-clock cap on the entire run. Defaults to
	// defaultDeadline (60s). Run() wraps ctx with this; pre-flight
	// checks fire at iteration boundaries.
	Deadline time.Duration

	// TraceID flows through every log line, every ai_usage row (when
	// F4 lands), every Temporal workflow id, and every Redis event
	// stream key. Caller MUST generate it before invoking Run().
	//
	// The runner treats an empty TraceID as a programmer error and
	// returns StatusFailed with a terminal error.
	TraceID string

	// EventPublisher overrides the runner's default publisher for this
	// call only. Nil falls back to Runner.Opts.Events.
	EventPublisher EventPublisher

	// InvokerUserID is the user the dispatcher audits tool calls under.
	// The HTTP handler populates from the session; in-process callers
	// (the future inbound pipeline) pass the System user id.
	// Must reference an existing users.id — Postgres FK enforces this.
	InvokerUserID int

	// IsComposer marks this as a composer run. The composer is a
	// platform component, NOT an ai_agents row, so when this is set the runner
	// uses the platform composer config (platformComposerAgent) instead
	// of loading AgentID from the DB, and injects call_subagent (given an
	// invoker). The composer's identity comes from how the run is started,
	// not from a row. See call_subagent.go.
	IsComposer bool

	// SubAgentInvoker, when non-nil on a composer run, lets the
	// composer call sub-agents via the call_subagent tool. The runner
	// injects the tool only when IsComposer && SubAgentInvoker != nil;
	// generic agents and every (recursively-invoked) sub-agent run leave both
	// unset — which both hides the tool and makes sub-agent recursion
	// impossible. Per-call override in the spirit of EventPublisher. See
	// call_subagent.go.
	SubAgentInvoker SubAgentInvoker

	// ApprovalRequiredSlugs is the set of custom-tool slugs that require
	// human approval before execution (resolved from the composer_config by
	// the caller). The gate is slug-driven and origin-agnostic: ANY run
	// (composer or sub-agent) that is about to dispatch a tool whose slug is
	// in this set stops with StatusAwaitingApproval instead of executing it.
	// See the gate in runner.go and Slice 4.
	ApprovalRequiredSlugs []string

	// Resume, when non-nil, makes this a RESUMING run: instead of starting
	// fresh, the runner reconstructs the prior context from Resume.PriorTrace,
	// resolves the previously-gated tool call (execute the approved write /
	// feed a rejection / inject a precomputed result), and continues the loop
	// from there — it does NOT re-reason its way back to the write. See the
	// resume path in runner.go and approval resume (Slice 4).
	Resume *ResumeContext
}

// ProposedWrite is the tool call an approval-gated run stopped before
// executing. It is the payload of a StatusAwaitingApproval run and the source
// of the persisted approval record. ToolID is the canonical ai_tools FK
// (resolved for free from the gated ResolvedTool); ToolSlug is what the LLM
// emitted and what the resuming dispatch uses; ToolUseID is the per-call LLM id.
type ProposedWrite struct {
	ToolID    int            `json:"tool_id"`
	ToolSlug  string         `json:"tool_slug"`
	ToolUseID string         `json:"tool_use_id"`
	Inputs    map[string]any `json:"inputs,omitempty"`

	// Persistence identity — set so the activity (the single persister) can
	// record the approval uniformly whether the gate fired in the top-level
	// run or bubbled up from a sub-agent. GatedTraceID is the trace of the run
	// that actually gated (top-level trace for an own gate; the sub-agent's
	// child trace when bubbled). AgentID is that run's agent (nil for the
	// orchestrator, which is not an ai_agents row). ParentTraceID is the
	// composer run a sub-agent gate belongs to (empty for an own gate); the
	// invoker fills it on bubble-up.
	GatedTraceID  string `json:"gated_trace_id,omitempty"`
	ParentTraceID string `json:"parent_trace_id,omitempty"`
	AgentID       *int   `json:"agent_id,omitempty"`
}

// ResumeContext drives a resuming run. PriorTrace is the gated run's persisted
// trace, rendered into a read-only transcript so the model has its prior
// context without replaying tool_use/thinking turns (avoids signature/pairing
// fragility). The runner then authors a single assistant(tool_use) + tool_result
// pair for the gated call and continues the loop. Exactly one resolution mode
// applies: ExecuteWrite (dispatch ToolName(Inputs) once and feed the output),
// Rejected (feed the rejection, no execution), Held (the write was approved but
// the situation changed during the approval window, so feed a neutral
// reconsider notice instead of executing — see HeldReason), or InjectedResult
// (feed the given content verbatim — used to feed a resumed sub-agent's report
// back into the composer at its call_subagent point).
type ResumeContext struct {
	PriorTrace []TurnTrace

	ToolName  string         // gated tool (write slug, or the call_subagent tool name)
	ToolUseID string         // original LLM tool_use id (informational; reconstruction uses a synthetic id)
	Inputs    map[string]any // inputs shown in the authored assistant tool_use block

	ExecuteWrite    bool    // dispatch ToolName(Inputs) once, feed its raw output
	Rejected        bool    // feed a rejection tool_result, no dispatch
	RejectionReason string  // rejection text fed back when Rejected
	Held            bool    // approved-but-held: feed a neutral reconsider notice, no dispatch
	HeldReason      string  // what changed during approval (fed back when Held)
	InjectedResult  *string // feed this content verbatim (no dispatch) — e.g. a resumed sub-agent report
}

// RunResponse is what Run() returns alongside its error. Even on
// failure the response is populated as much as possible — the test
// panel renders it so the operator can see what happened up to the
// failure point.
type RunResponse struct {
	// Draft is the customer-facing reply text committed via
	// produce_draft. Nil means the run never committed a draft and must
	// not be sent to customers.
	Draft *string `json:"draft"`

	// Status is the canonical outcome classification. Downstream
	// consumers branch on this.
	Status RunStatus `json:"status"`

	// PartialReason carries a short explanation when Status indicates
	// partial / incomplete / budget / deadline / max_iter / failed.
	// Empty on Status=complete.
	PartialReason string `json:"partial_reason,omitempty"`

	// Iterations is the number of LLM turns made. Always ≥ 1 unless
	// Status=failed before any turn completed.
	Iterations int `json:"iterations"`

	// ToolCalls is the flat per-turn-merged log of every tool
	// invocation. Same content shows up in Trace[i].ToolCalls; this
	// flat view is for callers that don't care about per-turn grouping.
	ToolCalls []ToolCallLog `json:"tool_calls,omitempty"`

	// Trace is the complete per-turn audit. The test panel renders
	// this as a timeline.
	Trace []TurnTrace `json:"trace"`

	// Usage is the aggregate token count across all turns.
	Usage llm.Usage `json:"usage"`

	// Metrics surfaces aggregate latency / cache stats.
	Metrics RunMetrics `json:"metrics"`

	// AgentSlug is echoed back so log scrapers don't have to join
	// against ai_agents.
	AgentSlug string `json:"agent_slug,omitempty"`

	// AgentUpdatedAt is the loaded agent's updated_at at the moment of
	// the run. The HTTP handler stamps this into
	// agent_executions.agent_updated_at so the Composer tab can later
	// detect stale runs (agent edited after the run was generated).
	// Zero on the failure paths that never load the agent.
	AgentUpdatedAt time.Time `json:"agent_updated_at,omitempty"`

	// Drops is non-nil when the resolver dropped tools at run time
	// (unknown id, empty kb_scope, disabled custom tool).
	// Surfaced so the operator can see why a tool the agent declared
	// wasn't available.
	Drops map[int]string `json:"drops,omitempty"`

	// Decision is the composer's terminal decision (ANSWER or
	// ESCALATE). Nil for non-composer runs (which terminate via
	// produce_draft) and for any run that failed before the composer
	// loop started. A successful composer run always carries one.
	// For ANSWER the customer reply lives in Draft; for ESCALATE the
	// internal reason lives here and any customer holding message in Draft.
	Decision *TerminalDecision `json:"decision,omitempty"`

	// IsComposer records whether the run was a composer run,
	// stamped once the agent loads. The activity boundary reads this on
	// the error path to decide whether a terminal failure should escalate
	// (fail-safe to a human) rather than drop the ticket. Zero on the
	// pre-load failure paths, which deliberately stay StatusFailed.
	IsComposer bool `json:"is_composer,omitempty"`

	// ProposedWrite is set only on Status=StatusAwaitingApproval: the
	// approval-gated tool call the run stopped before executing. The activity
	// persists it as a pending approval record; the resuming run executes it.
	ProposedWrite *ProposedWrite `json:"proposed_write,omitempty"`
}

// TerminalDecision is the composer's exit: ANSWER or ESCALATE, and
// nothing else (no complete/ask split, no confidence — see
// AI_LAYER_DESIGN.md §4). It is the platform-enforced terminal contract for
// a composer run: every successful composer run reaches exactly one,
// and the platform synthesizes an ESCALATE for any run that would otherwise
// end without a decision (the fail-safe).
type TerminalDecision struct {
	// Kind is DecisionKindAnswer or DecisionKindEscalate.
	Kind string `json:"kind"`

	// InternalReason is the human-facing reason a run was escalated.
	// Set only for DecisionKindEscalate; never shown to the customer.
	InternalReason string `json:"internal_reason,omitempty"`
}

// Terminal decision kinds. The composer ends with exactly one.
const (
	DecisionKindAnswer   = "answer"
	DecisionKindEscalate = "escalate"
)

// TurnTrace is the audit record of one LLM call within Run(). Captured
// before the loop reacts to stop_reason so partial failures still
// leave a trail.
type TurnTrace struct {
	TurnIndex int `json:"turn_index"`

	// StopReason is the provider's reason for ending the turn. Mirrors
	// llm.StopReason values; unknown reasons flow through as-is so ops
	// can spot new Anthropic states.
	StopReason llm.StopReason `json:"stop_reason"`

	// Text is whatever plain text content the assistant emitted this
	// turn. Thinking is stored separately in ThinkingBlocks.
	Text string `json:"text,omitempty"`

	// ThinkingBlocks preserves structured thinking content from this
	// turn's LLM response, including signatures required to round-trip
	// Anthropic thinking across subsequent turns.
	ThinkingBlocks []ThinkingBlockTrace `json:"thinking_blocks,omitempty"`

	// ToolCalls is the list of tool invocations dispatched at the end
	// of this turn. Empty on terminal turns (end_turn, max_tokens, etc).
	ToolCalls []ToolCallLog `json:"tool_calls,omitempty"`

	// LatencyMs is the wall-clock LLM call latency for this turn.
	LatencyMs int `json:"latency_ms"`

	// Usage is the token count for this turn alone.
	Usage llm.Usage `json:"usage"`
}

// ToolCallLog is the audit record of one tool invocation within a turn.
// Status mirrors the dispatcher's outcome; Result.Data carries whatever
// the capability or sandbox returned.
type ToolCallLog struct {
	// ToolKind is "internal", "custom", or "runner" for synthetic
	// platform-owned tools such as produce_draft.
	ToolKind string `json:"tool_kind"`

	// ToolID is the capability id for internal, ai_tools.slug for
	// custom. Stable across renames per F5 contract.
	ToolID string `json:"tool_id"`

	// ToolName is the operator-facing display name captured at dispatch
	// time. Built-ins source this from the capability registry; custom
	// tools use ai_tools.name. Empty when the tool couldn't be resolved.
	ToolName string `json:"tool_name,omitempty"`

	// Input is the parameters the LLM emitted (provider-supplied;
	// validated against the tool's input_schema before dispatch).
	Input map[string]any `json:"input,omitempty"`

	// Status is the outcome envelope. Dispatcher/runtime errors use
	// validation_error/execution_error/unavailable; SDK-controlled
	// non-success results preserve their kind (failure/error/noop).
	Status string `json:"status"`

	// Data is populated on Status=success with the tool's output
	// (already JSON-marshallable). Nil otherwise.
	Data any `json:"data,omitempty"`

	// Error carries the human-readable failure reason on non-success.
	// Same string the dispatcher returns from its sentinels.
	Error string `json:"error,omitempty"`

	// Code carries the SDK/runner result code for non-success script results.
	Code string `json:"code,omitempty"`

	// Retryable mirrors sdk.error(..., retryable=True) for LLM planning.
	Retryable bool `json:"retryable,omitempty"`

	// StartedAt is when dispatch entered Execute. Captured by
	// dispatchOneTool. Stable across parallel execution; downstream
	// consumers use this with FinishedAt to visualize overlap.
	StartedAt time.Time `json:"started_at"`

	// FinishedAt is when Execute returned (success or error).
	FinishedAt time.Time `json:"finished_at"`

	// DurationMs is FinishedAt.Sub(StartedAt) in milliseconds. Stays
	// for backwards compatibility with existing trace consumers; new
	// consumers should prefer StartedAt/FinishedAt.
	DurationMs int64 `json:"duration_ms"`

	// Recovered is true when dispatchOneTool's defer recover() caught
	// a panic from the tool's code. Distinct from a regular execution
	// error — surfaces "the tool's implementation has a bug" to ops.
	Recovered bool `json:"recovered,omitempty"`

	// ExecutionID is the `function_executions.execution_id` value that
	// audit row writes; lets ops join the trace with the audit table.
	ExecutionID string `json:"execution_id,omitempty"`
}

// ThinkingBlockTrace is the trace-serialized shape of one LLM thinking
// block. Visible thinking and redacted thinking are preserved
// separately so signatures survive round-trip and UIs can distinguish
// readable from opaque content.
type ThinkingBlockTrace struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

// RunMetrics aggregates timing + caching stats for the whole run.
// LatencyMs ≠ Sum(LLMLatencyMs + ToolLatencyMs) because tool dispatch
// happens between LLM calls but they're not exhaustive — some wall
// time is taken by acquire/release of internal locks.
type RunMetrics struct {
	TotalLatencyMs int `json:"total_latency_ms"`
	LLMLatencyMs   int `json:"llm_latency_ms"`
	ToolLatencyMs  int `json:"tool_latency_ms"`

	// CacheHits is the number of turns where Anthropic reported a
	// non-zero cache_read_input_tokens. CachedTokens is the sum.
	// Both feed cost dashboards once F4 lands; ops scan this to
	// confirm caching is working in production.
	CacheHits    int `json:"cache_hits"`
	CachedTokens int `json:"cached_tokens"`
}
