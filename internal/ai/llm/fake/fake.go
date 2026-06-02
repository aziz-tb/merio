// Package fake provides an in-memory llm.LLMProvider for tests. It
// supports static responses, ordered sequences, and error injection per
// purpose. Every call is recorded so assertions can inspect what the
// system under test actually sent.
//
// The fake is intentionally permissive: tests register the responses
// they need; anything they don't register fails the call with
// ErrFakeNotConfigured rather than returning a silent default. Silent
// defaults hide test bugs.
package fake

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// ErrFakeNotConfigured is returned when a Complete call hits a purpose
// that has no registered response, sequence, or error. The message
// includes the purpose name so test failures point at the missing
// configuration immediately.
var ErrFakeNotConfigured = errors.New("fake: no response configured for purpose")

// Provider is an in-memory llm.LLMProvider for tests.
//
// Resolution order on Complete:
//  1. If errors[purpose] has entries, pop the head and return that error.
//  2. If sequence[purpose] has entries, pop the head and return its response.
//  3. If static[purpose] is set, return that response.
//  4. Otherwise, return ErrFakeNotConfigured.
//
// Concurrency-safe — internal map access is guarded by a single mutex.
// Tests that race Register* against Complete should serialize themselves.
type Provider struct {
	mu       sync.Mutex
	static   map[llm.Purpose]llm.CompletionResponse
	sequence map[llm.Purpose][]llm.CompletionResponse
	errors   map[llm.Purpose][]error
	calls    []llm.CompletionRequest
}

// New builds an empty fake. Register* methods populate it.
func New() *Provider {
	return &Provider{
		static:   map[llm.Purpose]llm.CompletionResponse{},
		sequence: map[llm.Purpose][]llm.CompletionResponse{},
		errors:   map[llm.Purpose][]error{},
		calls:    nil,
	}
}

// Name reports the provider identifier. Distinct from "anthropic" so log
// scrapers don't confuse fake calls with production traffic.
func (p *Provider) Name() string { return "fake" }

// RegisterStatic configures a single response that's returned for every
// call to purpose. Overrides any prior static configuration; doesn't
// affect sequence or errors for the same purpose.
func (p *Provider) RegisterStatic(purpose llm.Purpose, resp llm.CompletionResponse) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.static[purpose] = resp
}

// RegisterSequence configures an ordered list of responses for purpose;
// each Complete call consumes the head. After the sequence is exhausted,
// resolution falls through to static / error.
func (p *Provider) RegisterSequence(purpose llm.Purpose, resps []llm.CompletionResponse) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]llm.CompletionResponse, len(resps))
	copy(cp, resps)
	p.sequence[purpose] = cp
}

// RegisterError appends a one-shot error to purpose's error queue. The
// next Complete call to purpose returns this error and pops it; multiple
// calls to RegisterError stack errors in FIFO order.
func (p *Provider) RegisterError(purpose llm.Purpose, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors[purpose] = append(p.errors[purpose], err)
}

// ClearAll resets all registered responses, sequences, errors, and call
// recordings. Useful for tests that share a fixture across subtests.
func (p *Provider) ClearAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.static = map[llm.Purpose]llm.CompletionResponse{}
	p.sequence = map[llm.Purpose][]llm.CompletionResponse{}
	p.errors = map[llm.Purpose][]error{}
	p.calls = nil
}

// Calls returns a snapshot of every CompletionRequest the fake has been
// called with, in chronological order. Returned slice is a copy.
func (p *Provider) Calls() []llm.CompletionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]llm.CompletionRequest, len(p.calls))
	copy(out, p.calls)
	return out
}

// CallsForPurpose filters Calls() by purpose.
func (p *Provider) CallsForPurpose(purpose llm.Purpose) []llm.CompletionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]llm.CompletionRequest, 0)
	for _, c := range p.calls {
		if c.Purpose == purpose {
			out = append(out, c)
		}
	}
	return out
}

// CallCount returns the total number of Complete calls observed.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// Complete consumes the next configured response for req.Purpose per the
// resolution order in the package doc. Honors ctx cancellation.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.CompletionResponse{}, err
	}

	p.mu.Lock()
	p.calls = append(p.calls, req)

	// 1. errors queue
	if errs := p.errors[req.Purpose]; len(errs) > 0 {
		head := errs[0]
		p.errors[req.Purpose] = errs[1:]
		p.mu.Unlock()
		return llm.CompletionResponse{}, head
	}

	// 2. sequence queue
	if seq := p.sequence[req.Purpose]; len(seq) > 0 {
		head := seq[0]
		p.sequence[req.Purpose] = seq[1:]
		p.mu.Unlock()
		return p.stamp(head, req), nil
	}

	// 3. static
	if resp, ok := p.static[req.Purpose]; ok {
		p.mu.Unlock()
		return p.stamp(resp, req), nil
	}

	p.mu.Unlock()
	return llm.CompletionResponse{}, fmt.Errorf("%w: %q", ErrFakeNotConfigured, req.Purpose)
}

// StreamComplete returns ErrStreamingNotImplemented to mirror the real
// provider's v1 behavior. Tests for streaming logic should expect this
// error rather than fake streaming.
func (p *Provider) StreamComplete(_ context.Context, _ llm.CompletionRequest, _ func(string)) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, llm.ErrStreamingNotImplemented
}

// stamp fills in PresetUsed/Model from the request's PresetSlug if the
// configured response left them empty. Lets tests register a partial
// CompletionResponse without restating boilerplate every time.
func (p *Provider) stamp(resp llm.CompletionResponse, req llm.CompletionRequest) llm.CompletionResponse {
	if resp.PresetUsed == "" {
		resp.PresetUsed = req.PresetSlug
	}
	return resp
}
