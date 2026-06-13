package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	enumspb "go.temporal.io/api/enums/v1"

	kmodels "github.com/aziz-tb/merio/internal/knowledge/models"
	"github.com/aziz-tb/merio/internal/temporal"
	wfembeddings "github.com/aziz-tb/merio/internal/workflows/embeddings"
)

// kickEmbedIfPublished triggers EmbedArticleWorkflow when the article is
// published; no-ops otherwise. Does NOT check is_ai_groundable (agent
// kb_scope is the authoritative gate; per F12 fork-5). Fire-and-forget with
// a short dispatch timeout: on failure it logs and falls back to
// MarkIndexStale, never failing the save.
func (app *App) kickEmbedIfPublished(article kmodels.Article) {
	if article.Status != "published" {
		return
	}
	if app.temporal == nil {
		// Temporal disabled: mark stale so the catch-up sweep re-embeds.
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

// unembedArticle deletes an article's vectors synchronously; invoked from the
// Unpublish and DeleteArticle handlers. On failure it logs and marks the
// index stale for the catch-up sweep, never failing the handler.
func (app *App) unembedArticle(articleID int) {
	if app.searcher == nil {
		// Hybrid-search subsystem not configured; nothing to clean up.
		return
	}
	if app.embeddingsStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.embeddingsStore.DeleteArticle(ctx, int64(articleID)); err != nil {
			app.lo.Error("failed to delete article vectors", "article_id", articleID, "error", err)
			app.knowledge.MarkIndexStale(articleID)
		}
	}
}
