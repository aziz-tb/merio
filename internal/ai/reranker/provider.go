// Package reranker defines the provider-agnostic surface for cross-encoder
// reranking — stage 3 of the hybrid search pipeline. Separate from the
// embedding provider because the API endpoint, pricing, and likely future
// vendor are independent (Cohere has a strong reranker but a different
// embedding lineup; vice versa for OpenAI). Implementations:
// internal/ai/reranker/voyage (production), future fakes for tests.
package reranker

import "context"

// Provider is the cross-encoder reranking interface. Scores each
// candidate against the query, returns results sorted by relevance.
type Provider interface {
	// Rerank ranks `candidates` against `query` and returns up to topK
	// results in descending relevance order. Indexes in the result
	// slice point back into the input `candidates` slice; callers map
	// them onto their richer per-candidate metadata.
	Rerank(ctx context.Context, query string, candidates []string, topK int) ([]Result, error)

	// Model returns the canonical project-namespaced model id.
	Model() string

	// Name is the short provider identifier ("voyage", "cohere") for
	// logs / metrics.
	Name() string
}

// Result is one ranked candidate.
type Result struct {
	// Index is the position in the input candidates slice.
	Index int

	// RelevanceScore is the provider-emitted score, typically in [0,1].
	// Higher = more relevant. Absolute values are not comparable across
	// providers; rank order is.
	RelevanceScore float64
}
