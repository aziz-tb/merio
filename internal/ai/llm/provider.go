package llm

import "context"

// LLMProvider is the provider-agnostic interface that everything calling
// an LLM goes through. v1 production providers are Anthropic, OpenAI, and
// Azure; FakeLLMProvider covers tests. Future provider implementations slot
// in with no caller-side changes.
//
// The interface is deliberately small: Complete is the only method
// callers reach for in v1. StreamComplete is reserved for L5 auto-send
// streaming once that lands; v1 implementations return ErrStreamingNotImplemented.
type LLMProvider interface {
	// Complete invokes the LLM with the given request and returns the
	// assistant's response. The provider is responsible for retries on
	// transient errors, circuit breaking on repeated failure, bounded
	// concurrency, and prompt caching. Callers may treat any returned
	// error as terminal (the provider has already exhausted its policy).
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// StreamComplete invokes the LLM in streaming mode, calling onChunk
	// with each text delta as it arrives. Returns ErrStreamingNotImplemented
	// in v1 — wired for the agent runtime once L5 auto-send needs it.
	//
	// The final CompletionResponse is returned at the end of the stream;
	// onChunk only receives text deltas (tool_use blocks are not streamed
	// in v1).
	StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(string)) (CompletionResponse, error)

	// Name returns a stable provider identifier ("anthropic", "openai",
	// "azure", "fake").
	// Used in log fields, metric labels, and ai_usage.provider.
	Name() string
}
