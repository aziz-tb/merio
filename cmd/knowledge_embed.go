package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	enumspb "go.temporal.io/api/enums/v1"

	kmodels "github.com/abhinavxd/libredesk/internal/knowledge/models"
	"github.com/abhinavxd/libredesk/internal/temporal"
	wfembeddings "github.com/abhinavxd/libredesk/internal/workflows/embeddings"
)

// kickEmbedIfPublished is the unified hook called by the KB article
// save handlers (create, update, publish). Triggers
// EmbedArticleWorkflow when the article is currently published; no-ops
// otherwise. Failures fall back to MarkIndexStale so the catch-up
// sweep eventually retries.
//
// Per F12 fork-5 resolution this function does NOT check
// is_ai_groundable — agent kb_scope is the authoritative gate for
// what each agent can see. Embedding-everything-published is the v1
// stance.
//
// Fire-and-forget: dispatch is bounded by a short timeout (workflow
// START is cheap; long timeouts here would block the HTTP response).
// Errors are logged + recorded as stale; the handler does NOT fail
// the save.
func (app *App) kickEmbedIfPublished(article kmodels.Article) {
	if article.Status != "published" {
		return
	}
	if app.temporal == nil {
		// Temporal disabled: mark stale so the catch-up sweep (when
		// implemented) eventually re-embeds. Never silently no-op.
		app.knowledge.MarkIndexStale(article.ID)
		app.lo.Warn("Temporal disabled; article marked stale", "article_id", article.ID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workflowID := fmt.Sprintf("embed-article:%d:v%d", article.ID, article.Revision)
	_, err := app.temporal.StartWorkflow(ctx, temporal.StartOptions{
		ID:        workflowID,
		TaskQueue: temporal.QueueEmbeddings,
		SearchAttributes: map[string]any{
			temporal.SAWorkflowKind: "embedding",
			temporal.SAEntityType:   "kb_article",
			temporal.SAEntityID:     strconv.Itoa(article.ID),
			temporal.SATraceID:      workflowID,
		},
		ReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
	}, wfembeddings.EmbedArticleWorkflow, wfembeddings.EmbedArticleInput{
		ArticleID: int64(article.ID),
		Revision:  article.Revision,
		TraceID:   workflowID,
	})
	if err != nil {
		app.lo.Error("failed to start embed workflow", "article_id", article.ID, "error", err)
		app.knowledge.MarkIndexStale(article.ID)
		return
	}
	app.lo.Info("embed workflow started", "article_id", article.ID, "revision", article.Revision, "workflow_id", workflowID)
}

// unembedArticle is invoked from Unpublish and DeleteArticle handlers.
// Triggers a one-shot deletion workflow OR (v1) calls the activity
// directly via a synthetic workflow id. v1 implementation: route
// through the same embeddings queue with a deterministic id; the
// worker registers DeleteArticleVectors as a registered activity that
// can be invoked via a one-step workflow shim.
//
// For F12 v1 we use a simpler path: directly call the Searcher's
// underlying Store via a small handler that runs synchronously. The
// rationale: unpublish is rare AND the operator already sees the UI
// hang on save; a workflow indirection here adds complexity without
// benefit.
//
// When Temporal is disabled or the Store call fails, the article's
// is_stale flag is set so the catch-up sweep eventually reconciles.
func (app *App) unembedArticle(articleID int) {
	if app.searcher == nil {
		// Hybrid-search subsystem not configured; nothing to clean up.
		return
	}
	// Synchronous DB call via the embeddings store. The store is held
	// by the searcher; we reach it via a small accessor pattern.
	// Keeping this synchronous because the handler is already serial.
	// Errors are logged + recorded as stale; the handler doesn't fail.
	if app.embeddingsStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.embeddingsStore.DeleteArticle(ctx, int64(articleID)); err != nil {
			app.lo.Error("failed to delete article vectors", "article_id", articleID, "error", err)
			app.knowledge.MarkIndexStale(articleID)
		}
	}
}
