package airuns

import (
	"context"
	"errors"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// This file is the ACTIVITY-BOUNDARY error policy for composer runs (the
// fail-safe's Layer 2). The runner's deferred normalizer (Layer 1) already
// escalates a composer loop that ended WITHOUT an error but without a
// decision; it deliberately leaves error returns untouched. Error returns are
// decided here, because retry disposition is a Temporal/activity concern — only
// the activity can read activity.GetInfo to know whether a retry is still
// pending. Keeping these as pure functions makes the policy unit-testable
// without a Temporal harness.

// errorDisposition is what the activity does with a terminal runner error.
type errorDisposition int

const (
	// dispositionFailLoud: surface the error to Temporal (which fails the
	// workflow, since it is the final attempt). The default for non-composer
	// runs, bugs/config errors, and anything unrecognized — never masked as a
	// successful escalation.
	dispositionFailLoud errorDisposition = iota

	// dispositionRetry: a Temporal retry is still pending, so return the error
	// and let the policy re-run the activity. Never escalate while a retry could
	// still resolve it — that would cannibalize the retry net.
	dispositionRetry

	// dispositionEscalate: a transient-class failure with retries exhausted.
	// The run genuinely cannot complete right now, so hand the ticket to a
	// human rather than drop it.
	dispositionEscalate
)

// composerErrorDisposition decides what to do with a terminal runner error
// on a composer run. The three rules it encodes:
//   - Transient errors must keep their Temporal retries: while willRetry is
//     true they return for retry, never escalate-on-first-occurrence.
//   - Bugs / config / unknown errors fail loud and stay Temporal-visible —
//     never masked as an escalation that would flood the human queue.
//   - Only a terminal transient failure (transient class, no retry pending)
//     escalates, so the ticket reaches a human instead of being dropped.
func composerErrorDisposition(err error, isComposer bool, willRetry bool) errorDisposition {
	if err == nil || !isComposer {
		// Non-composer runs keep today's behavior: any error fails loud.
		return dispositionFailLoud
	}
	if willRetry {
		return dispositionRetry
	}
	if isTransientProviderError(err) {
		return dispositionEscalate
	}
	return dispositionFailLoud
}

// isTransientProviderError reports whether err is one of the provider error
// classes the LLM client *explicitly* surfaces as a transient, retryable
// sentinel. The list is intentionally limited to what the clients actually
// emit (internal/ai/llm/anthropic|openai|azure):
//
//   - ErrRateLimited  — 429 after the client's own retries were exhausted.
//   - ErrCircuitOpen  — breaker open (fast-fail; the breaker will close).
//   - ErrTimeout      — defined for forward-compat; currently UNEMITTED (kept
//     so the moment a client wraps per-call timeouts with it, it classifies
//     correctly without a code change).
//   - context.DeadlineExceeded — a per-call wall-clock timeout.
//
// Deliberately NOT transient here, so they fall through to fail-loud (the safe
// direction — better to fail loud than wrongly escalate):
//   - 5xx server errors and network/DNS/TLS errors: the client surfaces these
//     as RAW errors, not sentinels (anthropic/retry.go), so errors.Is can't
//     detect them. Outage-shaped; failing loud avoids flooding the human queue.
//   - ErrAuth, ErrInvalidRequest: credential/request-shape bugs — config/code
//     problems that a human escalation can't fix.
//   - context.Canceled: shutdown/abort, not a ticket the AI couldn't handle.
func isTransientProviderError(err error) bool {
	switch {
	case errors.Is(err, llm.ErrRateLimited),
		errors.Is(err, llm.ErrCircuitOpen),
		errors.Is(err, llm.ErrTimeout),
		errors.Is(err, context.DeadlineExceeded):
		return true
	default:
		return false
	}
}
