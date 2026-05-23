// Package fake provides a deterministic embeddings.Provider for tests.
// Returns a hash-derived vector per input so the same string maps to
// the same vector across runs without an external API call.
package fake

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// Provider is a deterministic in-memory embedding provider for tests.
type Provider struct {
	model      string
	dims       int
	maxBatch   int
	failNext   error
	callCount  int
}

// New returns a fake provider configured for the given dimensions. Pass
// 1024 to match production halfvec(1024) storage.
func New(dims int) *Provider {
	return &Provider{
		model:    "fake/test@v1",
		dims:     dims,
		maxBatch: 128,
	}
}

// Embed returns deterministic vectors derived from each input string.
// Same input → same output across calls and across processes. Useful
// for assertions in integration tests without committing to a specific
// numeric vector layout.
func (p *Provider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	p.callCount++
	if p.failNext != nil {
		err := p.failNext
		p.failNext = nil
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = vectorFor(t, p.dims)
	}
	return out, nil
}

func (p *Provider) Model() string     { return p.model }
func (p *Provider) Dimensions() int   { return p.dims }
func (p *Provider) MaxBatchSize() int { return p.maxBatch }
func (p *Provider) Name() string      { return "fake" }

// FailNext primes the next Embed call to return err. Useful for
// exercising error paths in tests.
func (p *Provider) FailNext(err error) { p.failNext = err }

// CallCount returns how many times Embed has been invoked. Useful for
// asserting cache hit / miss in tests.
func (p *Provider) CallCount() int { return p.callCount }

// vectorFor produces a deterministic dims-length float32 vector from
// the input text by repeatedly hashing and seeding 4 bytes at a time
// from the hash digest. The output is normalized roughly to [-1, 1]
// — exact distribution isn't important, only that same input ⇒ same
// output.
func vectorFor(text string, dims int) []float32 {
	seed := sha256.Sum256([]byte(text))
	out := make([]float32, dims)
	for i := 0; i < dims; i++ {
		// Reseed every 8 floats by hashing the previous block; keeps
		// the sequence diverse for long vectors.
		if i%8 == 0 && i > 0 {
			seed = sha256.Sum256(seed[:])
		}
		off := (i % 8) * 4
		u := binary.LittleEndian.Uint32(seed[off : off+4])
		out[i] = (float32(u%2001) - 1000) / 1000.0
	}
	return out
}
