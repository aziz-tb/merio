package embeddings

import (
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	actembeddings "github.com/aziz-tb/merio/internal/activities/embeddings"
)

// BulkReindexInput drives a full re-indexing of one content type.
type BulkReindexInput struct {
	ContentType string // 'kb_chunk' (v1)
	BatchSize   int    // article ids fetched per page (default 100)
	Parallelism int    // concurrent child workflows in flight (default 4)
	TraceID     string
}

// BulkReindexOutput summarizes the run.
type BulkReindexOutput struct {
	Total     int
	Succeeded int
	Failed    int
}

// BulkReindexWorkflow pages through all published articles, running a
// per-article EmbedArticleWorkflow as a child. A token channel bounds
// concurrency to in.Parallelism children at once.
//
// Resumability: child workflow ids are deterministic, so restarting a
// terminated run re-embeds nothing new (already-embedded articles hit
// ON CONFLICT DO NOTHING and return fast).
func BulkReindexWorkflow(ctx workflow.Context, in BulkReindexInput) (*BulkReindexOutput, error) {
	logger := workflow.GetLogger(ctx)
	if in.BatchSize <= 0 {
		in.BatchSize = 100
	}
	if in.Parallelism <= 0 {
		in.Parallelism = 4
	}
	if in.ContentType == "" {
		in.ContentType = "kb_chunk"
	}

	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			MaximumInterval:    30 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)

	out := &BulkReindexOutput{}
	offset := 0

	for {
		var page actembeddings.ListPublishedArticlesOutput
		var acts *actembeddings.Activities
		if err := workflow.ExecuteActivity(ctx, acts.ListPublishedArticles, &actembeddings.ListPublishedArticlesInput{
			Limit:  in.BatchSize,
			Offset: offset,
		}).Get(ctx, &page); err != nil {
			return out, fmt.Errorf("bulk-reindex: list: %w", err)
		}
		if len(page.IDs) == 0 {
			break
		}

		// Bounded concurrent child workflows.
		sem := workflow.NewBufferedChannel(ctx, in.Parallelism)
		futures := make([]workflow.ChildWorkflowFuture, 0, len(page.IDs))
		for _, articleID := range page.IDs {
			sem.Send(ctx, struct{}{})
			articleID := articleID
			childOpts := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowID:            fmt.Sprintf("embed-article:%d:bulk", articleID),
				WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
			})
			fut := workflow.ExecuteChildWorkflow(childOpts, EmbedArticleWorkflow, EmbedArticleInput{
				ArticleID: articleID,
				Revision:  1, // synthetic revision for bulk; idempotent at the row level
				TraceID:   in.TraceID,
			})
			futures = append(futures, fut)
			// Drain the semaphore as each child resolves.
			workflow.Go(ctx, func(ctx workflow.Context) {
				var dummy any
				_ = fut.Get(ctx, &dummy)
				var token struct{}
				sem.Receive(ctx, &token)
			})
		}

		out.Total += len(page.IDs)
		for _, f := range futures {
			var dummy any
			if err := f.Get(ctx, &dummy); err != nil {
				out.Failed++
				logger.Warn("bulk-reindex child failed", "error", err)
			} else {
				out.Succeeded++
			}
		}
		if len(page.IDs) < in.BatchSize {
			break
		}
		offset += in.BatchSize
	}

	logger.Info("BulkReindexWorkflow done",
		"total", out.Total, "succeeded", out.Succeeded, "failed", out.Failed)
	return out, nil
}
