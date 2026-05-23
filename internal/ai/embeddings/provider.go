// Package embeddings defines the provider-agnostic surface for vector
// embedding generation + retrieval, plus the hybrid-search coordinator
// that glues semantic + keyword + reranking into one pipeline. Provider
// implementations live in subpackages (voyage, fake); workflow + activity
// code that drives indexing lives under internal/workflows/embeddings/
// and internal/activities/embeddings/.
//
// Design rules:
//
//   - Embedding generation is provider-agnostic. Today: Voyage. Tomorrow:
//     OpenAI, Cohere, self-hosted. The Provider interface is the contract.
//   - Reranking is a separate abstraction (internal/ai/reranker) because
//     the API endpoint, pricing model, and likely future vendor diverge.
//   - Vector storage is pgvector; the schema is fixed by migration v2_7_8.
//   - PII redaction runs BEFORE the embedding call. Embedding the raw
//     customer message would land a PII-bearing vector in the DB that no
//     downstream redactor can clean. See activities/embeddings/redact.go.
package embeddings

import "context"

// Provider is the provider-agnostic interface for embedding generation.
// Implementations: voyage.Provider (production), fake.Provider (tests).
type Provider interface {
	// Embed produces one embedding per input text. The provider is
	// responsible for chunking calls when len(texts) > MaxBatchSize();
	// callers always see one slice of vectors aligned with the input.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Model returns the canonical model identifier in the format
	// "<provider>/<model>@<version-tag>" (e.g.,
	// "voyage/voyage-3-large@2025-01"). This string is stored alongside
	// every vector and on ai_usage rows so migrations and cost analysis
	// can pin to specific model variants.
	Model() string

	// Dimensions returns the embedding vector size produced by this
	// provider/model. Must match the column type on the embeddings table
	// (currently halfvec(1024); a provider returning a different
	// dimension would fail at insert time).
	Dimensions() int

	// MaxBatchSize is the per-call cap on input batch size. Callers that
	// build their own batches use this; Embed itself handles internal
	// batching transparently.
	MaxBatchSize() int

	// Name is the short provider identifier ("voyage", "openai") used in
	// logs, metrics, and ai_usage.provider.
	Name() string
}
