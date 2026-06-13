// Package embeddings holds the Temporal workflows for KB embedding
// indexing. Pure orchestration — every DB / API / file system touch is
// an activity (see internal/activities/embeddings).
//
// Workflow IDs follow the convention from internal/temporal/CONVENTIONS.md:
//   - EmbedArticleWorkflow  → "embed-article:<id>:v<revision>"
//   - CleanupOldVectorsWorkflow → "cleanup-vectors:<id>:v<superseded>"
//   - BulkReindexWorkflow → "bulk-reindex:<content_type>:<model>"
//
// IDs include the revision so re-runs are safe; ON CONFLICT DO NOTHING
// on the embeddings table backstops idempotency at the row level.
package embeddings

import (
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	actembeddings "github.com/aziz-tb/merio/internal/activities/embeddings"
)

// EmbedArticleInput drives the per-article indexing workflow.
type EmbedArticleInput struct {
	ArticleID int64
	Revision  int    // source revision being indexed; becomes source_version
	TraceID   string // correlates with ai_usage + Temporal search attribute
}

// EmbedArticleOutput summarizes the run for observability.
type EmbedArticleOutput struct {
	ChunksEmbedded int
	EmbeddingModel string
}

// EmbedArticleWorkflow indexes one KB article into the embeddings
// table. Stages:
//
//  1. FetchSourceContent — load article body + locale.
//  2. ChunkContent       — markdown-aware split per locked F12 spec.
//  3. RedactText         — PII redaction over each chunk (loops).
//  4. GenerateEmbeddings — Voyage call (provider handles batching).
//  5. StoreEmbeddings    — INSERT ... ON CONFLICT DO NOTHING.
//  6. SwapActiveVersion  — atomic flip on articles_index_state.
//  7. CleanupOldVectors (child workflow, ABANDON parent close policy)
//     — schedules durable 7-day deletion of the superseded version.
//
// Re-running is safe: existing rows hit ON CONFLICT, and step 7's
// ALLOW_DUPLICATE_FAILED_ONLY keeps the cleanup child idempotent.
func EmbedArticleWorkflow(ctx workflow.Context, in EmbedArticleInput) (*EmbedArticleOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("EmbedArticleWorkflow", "article_id", in.ArticleID, "revision", in.Revision)

	// Retry policy tuned for transient Voyage (5xx / 429) and DB errors.
	opts := workflow.ActivityOptions{
		StartToCloseTimeout:    60 * time.Second,
		ScheduleToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			MaximumInterval:    30 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)

	// 1. Fetch.
	var src actembeddings.FetchSourceOutput
	var acts *actembeddings.Activities // typed nil for symbol lookup
	if err := workflow.ExecuteActivity(ctx, acts.FetchSourceContent, &actembeddings.FetchSourceInput{
		ContentType: "kb_chunk",
		SourceID:    in.ArticleID,
	}).Get(ctx, &src); err != nil {
		return nil, fmt.Errorf("fetch source: %w", err)
	}

	// 2. Chunk.
	var chunks actembeddings.ChunkOutput
	if err := workflow.ExecuteActivity(ctx, acts.ChunkContent, &actembeddings.ChunkInput{
		Text:   src.Body,
		Format: "markdown",
	}).Get(ctx, &chunks); err != nil {
		return nil, fmt.Errorf("chunk: %w", err)
	}
	if len(chunks.Chunks) == 0 {
		logger.Warn("article produced zero chunks; skipping embed", "article_id", in.ArticleID)
		return &EmbedArticleOutput{}, nil
	}

	// 3. Redact every chunk before embedding. Serial is fine: each call
	// is a microsecond regex pass; the activity boundary exists for
	// testability and retry granularity.
	redacted := make([]string, len(chunks.Chunks))
	for i, c := range chunks.Chunks {
		var r actembeddings.RedactOutput
		if err := workflow.ExecuteActivity(ctx, acts.RedactText, &actembeddings.RedactInput{Text: c.Text}).Get(ctx, &r); err != nil {
			return nil, fmt.Errorf("redact chunk %d: %w", i, err)
		}
		redacted[i] = r.Text
	}

	// 4. Generate embeddings. Voyage batches internally past MaxBatchSize.
	var emb actembeddings.GenerateEmbeddingsOutput
	if err := workflow.ExecuteActivity(ctx, acts.GenerateEmbeddings, &actembeddings.GenerateEmbeddingsInput{
		Texts:   redacted,
		TraceID: in.TraceID,
	}).Get(ctx, &emb); err != nil {
		return nil, fmt.Errorf("generate embeddings: %w", err)
	}

	// 5. Persist (ON CONFLICT DO NOTHING in SQL).
	var stored actembeddings.StoreEmbeddingsOutput
	if err := workflow.ExecuteActivity(ctx, acts.StoreEmbeddings, &actembeddings.StoreEmbeddingsInput{
		ContentType:    "kb_chunk",
		SourceID:       in.ArticleID,
		SourceVersion:  in.Revision,
		EmbeddingModel: emb.Model,
		Chunks:         chunks.Chunks,
		Vectors:        emb.Vectors,
	}).Get(ctx, &stored); err != nil {
		return nil, fmt.Errorf("store embeddings: %w", err)
	}

	// 6. Flip active version.
	if err := workflow.ExecuteActivity(ctx, acts.SwapActiveVersion, &actembeddings.SwapActiveVersionInput{
		ArticleID:   in.ArticleID,
		NewVersion:  in.Revision,
		ActiveModel: emb.Model,
	}).Get(ctx, nil); err != nil {
		return nil, fmt.Errorf("swap version: %w", err)
	}

	// 7. Schedule cleanup of the superseded version (revision-1); skipped
	// at revision=1, which has no predecessor. ABANDON close policy lets
	// cleanup outlive a terminated EmbedArticleWorkflow.
	if in.Revision > 1 {
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID:            fmt.Sprintf("cleanup-vectors:%d:v%d", in.ArticleID, in.Revision-1),
			ParentClosePolicy:     enumspb.PARENT_CLOSE_POLICY_ABANDON,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
		})
		// Fire-and-forget; not awaited.
		workflow.ExecuteChildWorkflow(childCtx, CleanupOldVectorsWorkflow, CleanupOldVectorsInput{
			ContentType:       "kb_chunk",
			SourceID:          in.ArticleID,
			SupersededVersion: in.Revision - 1,
			GraceDays:         7,
		})
	}

	return &EmbedArticleOutput{
		ChunksEmbedded: stored.Stored,
		EmbeddingModel: emb.Model,
	}, nil
}
