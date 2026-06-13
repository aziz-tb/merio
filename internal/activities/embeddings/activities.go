// Package embeddings holds the Temporal activities driving KB embedding
// indexing — the DB + provider work behind the orchestration in
// internal/workflows/embeddings. Activities are methods on Activities so
// deps are injected once at worker boot; each uses the typed
// pointer-to-struct input/output convention (internal/temporal/CONVENTIONS.md).
package embeddings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zerodha/logf"

	activitydeps "github.com/aziz-tb/merio/internal/activities/dependencies"
	"github.com/aziz-tb/merio/internal/ai/embeddings"
	"github.com/aziz-tb/merio/internal/ai/embeddings/chunking"
	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/usage"
	"github.com/aziz-tb/merio/internal/ai/usage/redact"
	"github.com/aziz-tb/merio/internal/knowledge"
)

// Activities groups every embedding-related activity, registered against
// the embeddings task queue at worker boot.
type Activities struct {
	Store     *embeddings.Store
	Provider  embeddings.Provider
	Knowledge *knowledge.Manager
	Usage     *usage.Manager
	InboxID   int
	Lo        *logf.Logger
}

// New validates dependencies and returns an Activities. Erroring here
// fails worker boot rather than every workflow at runtime.
func New(a *Activities) (*Activities, error) {
	if a == nil {
		return nil, errors.New("embedding activities: nil receiver")
	}
	if a.Store == nil {
		return nil, errors.New("embedding activities: Store is required")
	}
	if a.Provider == nil {
		return nil, errors.New("embedding activities: Provider is required")
	}
	if a.Knowledge == nil {
		return nil, errors.New("embedding activities: Knowledge is required")
	}
	if a.Lo == nil {
		return nil, errors.New("embedding activities: Lo is required")
	}
	if a.InboxID <= 0 {
		return nil, errors.New("embedding activities: InboxID must be > 0 (system inbox)")
	}
	return a, nil
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	return New(&Activities{
		Store:     deps.EmbeddingStore,
		Provider:  deps.EmbeddingProvider,
		Knowledge: deps.Knowledge,
		Usage:     deps.Usage,
		InboxID:   deps.SystemInboxID,
		Lo:        deps.Logger("embedding-activities"),
	})
}

// FetchSourceInput / FetchSourceOutput is the (content_type, source_id) →
// (raw markdown body, locale) activity used at workflow stage 1.
type FetchSourceInput struct {
	ContentType string
	SourceID    int64
}

type FetchSourceOutput struct {
	Body     string
	Locale   string
	Title    string
	Revision int
}

// FetchSourceContent loads the article body that will be chunked. v1
// only handles content_type='kb_chunk'; other types error explicitly.
func (a *Activities) FetchSourceContent(_ context.Context, in *FetchSourceInput) (*FetchSourceOutput, error) {
	if in == nil {
		return nil, errors.New("FetchSourceContent: nil input")
	}
	if in.ContentType != "kb_chunk" {
		return nil, fmt.Errorf("FetchSourceContent: unsupported content_type %q", in.ContentType)
	}
	art, err := a.Knowledge.GetArticle(int(in.SourceID))
	if err != nil {
		return nil, fmt.Errorf("FetchSourceContent: get article %d: %w", in.SourceID, err)
	}
	return &FetchSourceOutput{
		Body:     art.BodyMarkdown,
		Locale:   art.Locale,
		Title:    art.Title,
		Revision: art.Revision,
	}, nil
}

// ChunkInput / ChunkOutput is the body → chunks transformation.
type ChunkInput struct {
	Text   string
	Format string // "markdown" or "atomic"
}

type ChunkOutput struct {
	Chunks []chunking.Chunk
}

// ChunkContent applies the locked F12 chunking strategy. Pure CPU, but
// kept an activity so the workflow stays orchestration-only.
func (a *Activities) ChunkContent(_ context.Context, in *ChunkInput) (*ChunkOutput, error) {
	if in == nil {
		return nil, errors.New("ChunkContent: nil input")
	}
	var chunks []chunking.Chunk
	switch in.Format {
	case "markdown":
		chunks = chunking.ChunkMarkdown(in.Text, chunking.DefaultConfig())
	case "atomic", "":
		chunks = chunking.ChunkAtomicText(in.Text, chunking.DefaultConfig())
	default:
		return nil, fmt.Errorf("ChunkContent: unknown format %q", in.Format)
	}
	return &ChunkOutput{Chunks: chunks}, nil
}

// RedactInput / RedactOutput runs the PII redactor over one chunk's text.
type RedactInput struct {
	Text string
}

type RedactOutput struct {
	Text string
}

// RedactText scrubs PII before the chunk reaches the embedding provider:
// a vector built from raw PII can leak it.
func (a *Activities) RedactText(_ context.Context, in *RedactInput) (*RedactOutput, error) {
	if in == nil {
		return nil, errors.New("RedactText: nil input")
	}
	return &RedactOutput{Text: redact.String(in.Text)}, nil
}

// GenerateEmbeddingsInput / GenerateEmbeddingsOutput drives the
// provider's Embed call + writes one ai_usage row per batch.
type GenerateEmbeddingsInput struct {
	Texts   []string
	TraceID string
}

type GenerateEmbeddingsOutput struct {
	Vectors [][]float32
	Model   string
}

// GenerateEmbeddings calls Provider.Embed and records the spend in
// ai_usage. The usage row is written on success AND failure so cost
// analysis sees both.
func (a *Activities) GenerateEmbeddings(ctx context.Context, in *GenerateEmbeddingsInput) (*GenerateEmbeddingsOutput, error) {
	if in == nil {
		return nil, errors.New("GenerateEmbeddings: nil input")
	}
	startedAt := time.Now()
	vecs, embedErr := a.Provider.Embed(ctx, in.Texts)
	durationMs := int(time.Since(startedAt).Milliseconds())

	tokens := 0
	for _, t := range in.Texts {
		tokens += chunking.EstimateTokens(t)
	}

	outcome := usage.OutcomeSuccess
	if embedErr != nil {
		outcome = outcomeFor(embedErr)
	}

	if a.Usage != nil {
		_ = a.Usage.Record(ctx, usage.Row{
			CreatedAt:           time.Now().UTC(),
			TraceID:             in.TraceID,
			CallSequenceInTrace: 1,
			InboxID:             a.InboxID,
			Provider:            a.Provider.Name(),
			Model:               a.Provider.Model(),
			PresetSlug:          "embedding",
			Purpose:             string(llm.PurposeEmbed),
			InputTokens:         tokens,
			OutputTokens:        0,
			LatencyMs:           durationMs,
			Outcome:             outcome,
		})
	}

	if embedErr != nil {
		return nil, embedErr
	}
	return &GenerateEmbeddingsOutput{Vectors: vecs, Model: a.Provider.Model()}, nil
}

// StoreEmbeddingsInput / StoreEmbeddingsOutput persists chunks +
// vectors against a specific source version.
type StoreEmbeddingsInput struct {
	ContentType    string
	SourceID       int64
	SourceVersion  int
	EmbeddingModel string
	Chunks         []chunking.Chunk
	Vectors        [][]float32
}

type StoreEmbeddingsOutput struct {
	Stored int
}

// StoreEmbeddings writes the new-version vectors. ON CONFLICT DO
// NOTHING in the INSERT statement makes retries safe.
func (a *Activities) StoreEmbeddings(ctx context.Context, in *StoreEmbeddingsInput) (*StoreEmbeddingsOutput, error) {
	if in == nil {
		return nil, errors.New("StoreEmbeddings: nil input")
	}
	if len(in.Chunks) != len(in.Vectors) {
		return nil, fmt.Errorf("StoreEmbeddings: %d chunks vs %d vectors", len(in.Chunks), len(in.Vectors))
	}
	stored := make([]embeddings.StoredChunk, len(in.Chunks))
	for i := range in.Chunks {
		stored[i] = embeddings.StoredChunk{Chunk: in.Chunks[i], Embedding: in.Vectors[i]}
	}
	if err := a.Store.StoreEmbeddings(ctx, in.ContentType, in.SourceID, in.SourceVersion, in.EmbeddingModel, stored); err != nil {
		return nil, err
	}
	return &StoreEmbeddingsOutput{Stored: len(stored)}, nil
}

// SwapActiveVersionInput / SwapActiveVersionOutput flips
// articles_index_state.active_version to make the new vectors
// queryable.
type SwapActiveVersionInput struct {
	ArticleID   int64
	NewVersion  int
	ActiveModel string
}

type SwapActiveVersionOutput struct{}

// SwapActiveVersion atomically flips the active-version pointer.
func (a *Activities) SwapActiveVersion(ctx context.Context, in *SwapActiveVersionInput) (*SwapActiveVersionOutput, error) {
	if in == nil {
		return nil, errors.New("SwapActiveVersion: nil input")
	}
	if err := a.Store.SwapActiveVersion(ctx, in.ArticleID, in.NewVersion, in.ActiveModel); err != nil {
		return nil, err
	}
	return &SwapActiveVersionOutput{}, nil
}

// GetIndexStateInput / Output are used by the cleanup workflow to
// confirm the active version still differs from the version it's
// about to delete (rollback safety).
type GetIndexStateInput struct {
	ArticleID int64
}

type GetIndexStateOutput struct {
	ActiveVersion int
	IsStale       bool
}

func (a *Activities) GetIndexState(ctx context.Context, in *GetIndexStateInput) (*GetIndexStateOutput, error) {
	if in == nil {
		return nil, errors.New("GetIndexState: nil input")
	}
	st, err := a.Store.GetIndexState(ctx, in.ArticleID)
	if err != nil {
		// Not-found is a normal case for cleanup of a never-indexed
		// article — return a zero-valued output rather than erroring.
		return &GetIndexStateOutput{}, nil
	}
	return &GetIndexStateOutput{ActiveVersion: st.ActiveVersion, IsStale: st.IsStale}, nil
}

// DeleteOldVectorsInput / Output is the cleanup workflow's terminal
// step: drop all rows for (content_type, source_id, version).
type DeleteOldVectorsInput struct {
	ContentType string
	SourceID    int64
	Version     int
}

type DeleteOldVectorsOutput struct{}

func (a *Activities) DeleteOldVectors(ctx context.Context, in *DeleteOldVectorsInput) (*DeleteOldVectorsOutput, error) {
	if in == nil {
		return nil, errors.New("DeleteOldVectors: nil input")
	}
	if err := a.Store.DeleteVersion(ctx, in.ContentType, in.SourceID, in.Version); err != nil {
		return nil, err
	}
	return &DeleteOldVectorsOutput{}, nil
}

// DeleteArticleVectorsInput / Output is the unpublish handler's
// activity — drops all versions for an article + its index-state row.
type DeleteArticleVectorsInput struct {
	ArticleID int64
}

type DeleteArticleVectorsOutput struct{}

func (a *Activities) DeleteArticleVectors(ctx context.Context, in *DeleteArticleVectorsInput) (*DeleteArticleVectorsOutput, error) {
	if in == nil {
		return nil, errors.New("DeleteArticleVectors: nil input")
	}
	if err := a.Store.DeleteArticle(ctx, in.ArticleID); err != nil {
		return nil, err
	}
	return &DeleteArticleVectorsOutput{}, nil
}

// ListPublishedArticlesInput / Output drives the bulk-reindex
// workflow's paging loop.
type ListPublishedArticlesInput struct {
	Limit  int
	Offset int
}

type ListPublishedArticlesOutput struct {
	IDs []int64
}

func (a *Activities) ListPublishedArticles(ctx context.Context, in *ListPublishedArticlesInput) (*ListPublishedArticlesOutput, error) {
	if in == nil {
		return nil, errors.New("ListPublishedArticles: nil input")
	}
	ids, err := a.Store.ListPublishedForReindex(ctx, in.Limit, in.Offset)
	if err != nil {
		return nil, err
	}
	return &ListPublishedArticlesOutput{IDs: ids}, nil
}

// outcomeFor maps embedding-provider errors onto the ai_usage outcome
// enum. Conservative: anything we can't classify lands in 'other'.
func outcomeFor(err error) string {
	if err == nil {
		return usage.OutcomeSuccess
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "circuit open"):
		return usage.OutcomeCircuitOpen
	case containsAny(msg, "context deadline", "timeout"):
		return usage.OutcomeTimeout
	case containsAny(msg, "status 401", "status 403"):
		return usage.OutcomeAuthError
	case containsAny(msg, "status 429", "rate limit"):
		return usage.OutcomeRateLimited
	case containsAny(msg, "status 4"):
		return usage.OutcomeInvalidRequest
	default:
		return usage.OutcomeOther
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) <= len(s) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
