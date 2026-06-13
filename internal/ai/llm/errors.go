package llm

import "errors"

// Sentinel errors returned by LLMProvider implementations. Callers
// distinguish behavior via errors.Is, not string matching.
var (
	// ErrStreamingNotImplemented — StreamComplete called on a provider
	// that doesn't support streaming. Returned by all v1 providers.
	ErrStreamingNotImplemented = errors.New("llm: streaming not implemented in v1")

	// ErrCircuitOpen — circuit breaker is open; surfaced without an
	// upstream call.
	ErrCircuitOpen = errors.New("llm: circuit breaker open")

	// ErrTimeout — call hit the provider's per-call wall-clock cap
	// (distinct from the SDK's HTTP deadline). Wraps the underlying ctx error.
	ErrTimeout = errors.New("llm: total call timeout exceeded")

	// ErrRateLimited — 429 after retries exhausted. Distinct from generic
	// transient errors so callers can throttle downstream work.
	ErrRateLimited = errors.New("llm: rate limited by provider")

	// ErrAuth — provider rejected the API key (401/403). Terminal; retries
	// do not help.
	ErrAuth = errors.New("llm: provider rejected credentials")

	// ErrInvalidRequest — request shape rejected by the provider (400).
	// Caller/translator bug, not a runtime issue.
	ErrInvalidRequest = errors.New("llm: provider rejected request shape")

	// ErrPresetNotFound — request's PresetSlug isn't in the loaded registry.
	// Fail-fast rather than falling back to a default.
	ErrPresetNotFound = errors.New("llm: preset slug not found in registry")

	// ErrEmptyMessages — request had no messages.
	ErrEmptyMessages = errors.New("llm: messages must not be empty")
)
