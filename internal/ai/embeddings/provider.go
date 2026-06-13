// Package embeddings defines the provider-agnostic surface for vector
// embedding generation + retrieval, plus the hybrid-search coordinator
// (semantic + keyword + reranking). Provider implementations live in
// subpackages (voyage, fake); indexing workflows/activities live under
// internal/workflows/embeddings/ and internal/activities/embeddings/.
//
// Design rules:
//
//   - The Provider interface is the contract for embedding generation.
//   - Reranking is a separate abstraction (internal/ai/reranker): its
//     endpoint, pricing, and likely future vendor diverge.
//   - Vector storage is pgvector; the schema is fixed by migration v2_7_8.
//   - PII redaction runs BEFORE the embedding call — otherwise the raw
//     customer message lands a PII-bearing vector in the DB that no
//     downstream redactor can clean. See activities/embeddings/redact.go.
package embeddings

import "context"

// Provider generates embeddings. Implementations: voyage.Provider
// (production), fake.Provider (tests).
type Provider interface {
	// Embed produces one embedding per input text, aligned with the input.
	// Embed batches internally when len(texts) > MaxBatchSize().
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Model returns the canonical identifier
	// "<provider>/<model>@<version-tag>" (e.g. "voyage/voyage-3-large@2025-01"),
	// stored on every vector and ai_usage row to pin migrations and cost
	// analysis to a specific model variant.
	Model() string

	// Dimensions returns the vector size; it must match the embeddings
	// column type (currently halfvec(1024)) or inserts fail.
	Dimensions() int

	// MaxBatchSize is the per-call input cap for callers that batch
	// themselves; Embed batches internally regardless.
	MaxBatchSize() int

	// Name is the short provider identifier ("voyage", "openai") used in
	// logs, metrics, and ai_usage.provider.
	Name() string
}
