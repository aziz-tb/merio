package llm

import "context"

// LLMProvider is the provider-agnostic interface every LLM call goes
// through. v1 production providers are Anthropic, OpenAI, and Azure;
// FakeLLMProvider covers tests. Complete is the only method callers reach
// for in v1; StreamComplete returns ErrStreamingNotImplemented.
type LLMProvider interface {
	// Complete invokes the LLM and returns the assistant's response. The
	// provider owns retries, circuit breaking, bounded concurrency, and
	// prompt caching, so callers may treat any returned error as terminal.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// StreamComplete invokes the LLM in streaming mode, calling onChunk with
	// each text delta (tool_use blocks are not streamed) and returning the
	// final CompletionResponse at the end. Returns ErrStreamingNotImplemented
	// in v1.
	StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(string)) (CompletionResponse, error)

	// Name returns a stable provider identifier ("anthropic", "openai",
	// "azure", "fake"), used in logs, metric labels, and ai_usage.provider.
	Name() string
}
