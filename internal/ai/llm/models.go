package llm

import (
	"encoding/json"
	"strings"
)

// Provider-agnostic request/response types. Provider implementations
// translate these to/from their wire format; nothing in this file imports
// from any provider SDK.

// Role is the speaker of a Message. Mirrors Anthropic's role taxonomy:
// system at the top of the conversation, then user/assistant pairs, with
// tool results embedded as ContentBlockToolResult inside user messages.
//
// Why "system" is a role here even though Anthropic carries the system
// prompt as a top-level field: callers build a single []Message slice;
// the Anthropic translator hoists Role=="system" into the SDK's system
// field. Other providers that accept a system role inline (OpenAI) can
// pass it through unchanged.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentBlockType identifies which variant of ContentBlock a block holds.
// Mirroring Anthropic's content-block taxonomy keeps the translator
// straightforward; other providers either accept the same shape or get
// flattened in their own translator.
type ContentBlockType string

const (
	ContentBlockTypeText             ContentBlockType = "text"
	ContentBlockTypeThinking         ContentBlockType = "thinking"
	ContentBlockTypeRedactedThinking ContentBlockType = "redacted_thinking"
	ContentBlockTypeToolUse          ContentBlockType = "tool_use"
	ContentBlockTypeToolResult       ContentBlockType = "tool_result"
)

// ContentBlock is one element inside a Message's content. A single message
// can mix text, thinking, redacted_thinking, tool_use (assistant invokes
// a tool), and tool_result blocks. Only fields relevant to the block's
// Type are populated; others are zero.
type ContentBlock struct {
	Type ContentBlockType `json:"type"`

	// Text — populated when Type == "text" or "thinking".
	Text string `json:"text,omitempty"`

	// Thinking metadata — populated when Type == "thinking" or
	// "redacted_thinking". Signature is required to round-trip visible
	// thinking blocks back to Anthropic on the next turn. Data carries
	// the opaque payload for redacted_thinking blocks.
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`

	// Tool use (assistant invoking a tool) — populated when Type == "tool_use".
	// ToolUseID is the provider-supplied id used to correlate the eventual
	// tool_result back to this call.
	ToolUseID string         `json:"tool_use_id,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`

	// Tool result (caller returning a tool's output) — populated when
	// Type == "tool_result". ToolUseID matches the assistant's earlier
	// tool_use block. Content is the tool's output as a string (callers
	// JSON-encode structured payloads). IsError signals the tool failed
	// so the model can surface the error or retry.
	ResultToolUseID string `json:"result_tool_use_id,omitempty"`
	ResultContent   string `json:"result_content,omitempty"`
	ResultIsError   bool   `json:"result_is_error,omitempty"`
}

// Message is one turn in the conversation. Content is a slice rather than
// a single string to support mixed-block messages (assistant text +
// tool_use, user message + tool_result attachments).
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ToolDef is the provider-agnostic shape of a tool the assistant may call.
// InputSchema is JSON Schema:
//
//	basic types, enum, nested objects, arrays of objects; no oneOf/anyOf, no format validators.
//
// Provider translators map this to the SDK's tool definition shape.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolChoice controls whether and which tool the provider must call.
// When nil (default), the model chooses freely between text and tool use.
//
// Type values:
//   - "auto" — model chooses (same as nil, default Anthropic behavior)
//   - "any"  — model must call some tool; model chooses which
//   - "tool" — model must call the named tool; Name field required
//
// Used by detection classifiers to force structured output via a specific
// tool. Also reusable by future structured-output LLM calls across the
// platform.
type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"` // required when Type == "tool"
}

// CachedBlockMarker records a position in the messages slice up to and
// including which the conversation prefix is "stable" and worth caching.
// AfterMessageIndex is the index of the last message that should be
// inside the cached span (so 0 means "cache the system prompt").
//
// The Anthropic translator places a cache_control marker on the last
// content block of the message at AfterMessageIndex. Up to 4 markers per
// request (Anthropic's cap); the cache manager in the agent runner
// enforces that.
type CachedBlockMarker struct {
	AfterMessageIndex int `json:"after_message_index"`
}

// CompletionRequest is the input to LLMProvider.Complete.
//
// PresetSlug is the resolved preset slug (caller resolves Purpose +
// agent overrides via the preset registry before calling). The provider
// uses it for logging and to look up model/temp/max_tokens from the
// in-memory preset map.
//
// CachedBlocks is optional. When present and the resolved preset has
// caching enabled, the provider adds cache_control markers; when absent
// or caching is disabled, no markers are added.
//
// TraceID flows through Redis Streams jobs, sandbox callbacks, and
// ai_usage rows so all LLM calls for a single ticket can be joined.
//
// AgentID and ResolutionAttemptID are zero outside their respective
// contexts; the rewrite UI, ad-hoc test programs, etc. pass zeros.
type CompletionRequest struct {
	Purpose    Purpose   `json:"purpose"`
	PresetSlug string    `json:"preset_slug"`
	Messages   []Message `json:"messages"`
	Tools      []ToolDef `json:"tools,omitempty"`
	// ToolChoice is optional. When nil, the model chooses freely. When set,
	// the provider translates to its forced-tool mechanism.
	ToolChoice          *ToolChoice         `json:"tool_choice,omitempty"`
	ThinkingEnabled     bool                `json:"thinking_enabled"`
	CachedBlocks        []CachedBlockMarker `json:"cached_blocks,omitempty"`
	TraceID             string              `json:"trace_id,omitempty"`
	AgentID             int                 `json:"agent_id,omitempty"`
	ResolutionAttemptID int                 `json:"resolution_attempt_id,omitempty"`
}

// StopReason is the provider's reason for ending the assistant's turn.
// Values are the canonical Anthropic strings; other providers translate
// to these. Unknown values flow through unchanged so ops can see them
// in logs.
type StopReason string

const (
	// StopReasonEndTurn — assistant finished naturally. The agent loop
	// still requires a terminal produce_draft tool call to commit the
	// customer-facing draft.
	StopReasonEndTurn StopReason = "end_turn"

	// StopReasonToolUse — assistant emitted one or more tool_use blocks;
	// the agent loop dispatches them and continues.
	StopReasonToolUse StopReason = "tool_use"

	// StopReasonMaxTokens — assistant hit the preset's max_tokens; the
	// response text is partial.
	StopReasonMaxTokens StopReason = "max_tokens"

	// StopReasonStopSequence — assistant hit a configured stop sequence;
	// treated as an intentional natural stop.
	StopReasonStopSequence StopReason = "stop_sequence"

	// StopReasonRefusal — Anthropic safety refusal (older models surface
	// this as a stop reason; newer models embed refusal in content).
	StopReasonRefusal StopReason = "refusal"

	// StopReasonPauseTurn — newer Anthropic models taking a thinking
	// break mid-turn. The agent loop appends and continues.
	StopReasonPauseTurn StopReason = "pause_turn"
)

// Usage tracks token consumption for one completion call.
//
// CachedTokens is broken out from InputTokens to make cache hit rate a
// direct query (Anthropic returns prompt-cache hit/miss counts as
// separate fields; we pre-aggregate into a single CachedTokens count).
//
// cached_tokens hits cost ~10% of regular input tokens —
// keeping the breakout makes the savings calculable downstream.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// CacheMetrics surfaces caching activity for one completion call. HitTokens
// echoes Usage.CachedTokens for readability at call sites that don't care
// about the rest of Usage. Markers is the count of cache_control markers
// the provider actually placed (may be less than the request's
// CachedBlocks count if the preset disabled caching or the provider
// capped at 4 markers).
type CacheMetrics struct {
	HitTokens int `json:"hit_tokens"`
	Markers   int `json:"markers"`
}

// CompletionResponse is the output of LLMProvider.Complete.
//
// PresetUsed and Model are echoed back so the caller can write a complete
// ai_usage row without re-resolving the preset. The agent runner writes
// these into TurnTrace; ai_usage instrumentation reads them when F4 lands.
type CompletionResponse struct {
	StopReason    StopReason            `json:"stop_reason"`
	Content       []ContentBlock        `json:"content"`
	Usage         Usage                 `json:"usage"`
	CacheMetrics  CacheMetrics          `json:"cache_metrics"`
	Provider      string                `json:"provider"`
	PresetUsed    string                `json:"preset_used"`
	Model         string                `json:"model"`
	Observability ResponseObservability `json:"observability"`
}

// ResponseObservability carries provider-side metrics the runner can't
// derive on its own — request IDs, retry counts, server-side timing,
// provider-specific cache-creation-vs-cache-read splits — and any
// other "squishy" provider data that doesn't deserve a typed column
// in CompletionResponse but does deserve to land in ai_usage.
//
// Every provider populates this on every return. Empty/zero values
// when the provider can't supply (fake provider in tests, etc.).
//
//   - ProviderRequestID — Anthropic msg.ID, OpenAI completion id, etc.
//     Persisted to ai_usage.provider_request_id for cross-system
//     debugging.
//   - RetryCount — number of attempts made before success/terminal
//     failure. 1 = no retries; 2+ = transient errors absorbed.
//     Persisted to ai_usage.retry_count.
//   - ProviderMetadata — JSON blob the provider can fill with anything
//     it judges useful. Anthropic uses this for the cache_creation /
//     cache_read split that doesn't fit in CacheMetrics. Persisted
//     verbatim to ai_usage.provider_metadata (no redaction — these
//     are operational metrics, not user content).
type ResponseObservability struct {
	Provider          string          `json:"provider,omitempty"`
	ProviderRequestID string          `json:"provider_request_id,omitempty"`
	RetryCount        int             `json:"retry_count,omitempty"`
	ProviderMetadata  json.RawMessage `json:"provider_metadata,omitempty"`
}

// ExtractText returns the concatenation of text blocks only. Thinking,
// redacted_thinking, and tool_* blocks are intentionally skipped so
// customer-facing drafts never absorb reasoning content by accident.
func ExtractText(content []ContentBlock) string {
	var parts []string
	for _, b := range content {
		if b.Type != ContentBlockTypeText {
			continue
		}
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n")
}

// ExtractThinkingBlocks returns all thinking-bearing blocks from the
// content in original order. Used by the runner for trace capture and
// by provider translators for round-trip continuity.
func ExtractThinkingBlocks(content []ContentBlock) []ContentBlock {
	out := make([]ContentBlock, 0)
	for _, b := range content {
		if b.Type == ContentBlockTypeThinking || b.Type == ContentBlockTypeRedactedThinking {
			out = append(out, b)
		}
	}
	return out
}

// ExtractThinkingText joins readable thinking text from visible
// thinking blocks. Redacted thinking blocks are skipped. This is for
// diagnostics/UI only; never use it for customer-facing drafts.
func ExtractThinkingText(content []ContentBlock) string {
	var parts []string
	for _, b := range content {
		if b.Type == ContentBlockTypeThinking && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ExtractToolUses returns the tool_use blocks from a response in the
// order the assistant emitted them. The agent runner dispatches them
// sequentially in this order.
func ExtractToolUses(content []ContentBlock) []ContentBlock {
	out := make([]ContentBlock, 0)
	for _, b := range content {
		if b.Type == ContentBlockTypeToolUse {
			out = append(out, b)
		}
	}
	return out
}
