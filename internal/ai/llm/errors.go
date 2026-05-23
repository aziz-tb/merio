package llm

import "errors"

// Sentinel errors returned by LLMProvider implementations. Callers
// distinguish behavior by errors.Is checks; they don't pattern-match on
// strings. New conditions get a new sentinel rather than overloading an
// existing one.
var (
	// ErrStreamingNotImplemented — StreamComplete called on a provider
	// that doesn't yet support streaming. v1 returns this from every
	// provider; revisit when L5 auto-send needs streaming.
	ErrStreamingNotImplemented = errors.New("llm: streaming not implemented in v1")

	// ErrCircuitOpen — the provider's circuit breaker is open. Surfaced
	// fast (no upstream call); typically the caller (agent runner) marks
	// the resolution_attempt as failed and the operator handles it.
	ErrCircuitOpen = errors.New("llm: circuit breaker open")

	// ErrTimeout — the call hit the provider's wall-clock cap (per-call
	// timeout, distinct from the SDK's HTTP deadline). Wraps the
	// underlying ctx error.
	ErrTimeout = errors.New("llm: total call timeout exceeded")

	// ErrRateLimited — provider returned 429 after retries exhausted.
	// Distinct from generic transient errors so the caller can rate-limit
	// downstream work or surface a dedicated message.
	ErrRateLimited = errors.New("llm: rate limited by provider")

	// ErrAuth — provider rejected the API key (401/403). Terminal —
	// retries do not help. Surface to ops via logs.
	ErrAuth = errors.New("llm: provider rejected credentials")

	// ErrInvalidRequest — request shape was rejected by the provider
	// (400). Caller bug, not a runtime issue. Caught by tests when the
	// translator gets out of sync with the SDK.
	ErrInvalidRequest = errors.New("llm: provider rejected request shape")

	// ErrPresetNotFound — caller passed a CompletionRequest whose
	// PresetSlug doesn't exist in the loaded preset registry. Provider
	// surfaces this as a fail-fast error rather than silently falling
	// back to a default; defaults are the registry's responsibility.
	ErrPresetNotFound = errors.New("llm: preset slug not found in registry")

	// ErrEmptyMessages — request had no messages. Defensive; caught here
	// rather than letting the SDK return a useless error.
	ErrEmptyMessages = errors.New("llm: messages must not be empty")
)
