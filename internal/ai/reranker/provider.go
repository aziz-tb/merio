// Package reranker is the provider-agnostic surface for cross-encoder
// reranking — stage 3 of the hybrid search pipeline. Kept separate from the
// embedding provider because endpoint, pricing, and vendor are independent.
package reranker

import "context"

// Provider is the cross-encoder reranking interface. Scores each
// candidate against the query, returns results sorted by relevance.
type Provider interface {
	// Rerank ranks candidates against query and returns up to topK results
	// in descending relevance order. Result.Index points back into the
	// input candidates slice.
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

	// RelevanceScore is the provider-emitted score (typically [0,1], higher
	// = more relevant). Not comparable across providers; only rank order is.
	RelevanceScore float64
}
