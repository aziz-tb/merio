package embeddings

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	actembeddings "github.com/aziz-tb/merio/internal/activities/embeddings"
)

// CleanupOldVectorsInput drives the durable-wait cleanup workflow.
type CleanupOldVectorsInput struct {
	ContentType       string
	SourceID          int64
	SupersededVersion int
	GraceDays         int
}

// CleanupOldVectorsWorkflow durably waits GraceDays then deletes the
// superseded version's vectors — but only if the operator hasn't
// rolled back to it in the meantime.
//
// This is the canonical Temporal "durable wait" pattern. workflow.Sleep
// survives worker restarts; a goroutine-based timer would lose the
// cleanup on any deploy.
//
// Idempotency: the workflow id is "cleanup-vectors:<id>:v<version>"
// (set by EmbedArticleWorkflow). Re-running deletes any remaining rows;
// already-deleted rows make the DELETE a no-op.
func CleanupOldVectorsWorkflow(ctx workflow.Context, in CleanupOldVectorsInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("CleanupOldVectorsWorkflow scheduled",
		"source_id", in.SourceID,
		"superseded_version", in.SupersededVersion,
		"grace_days", in.GraceDays)

	if in.GraceDays <= 0 {
		in.GraceDays = 7
	}

	// 1. Durable wait. workflow.Sleep is replay-safe and persists
	// across worker restarts.
	if err := workflow.Sleep(ctx, time.Duration(in.GraceDays)*24*time.Hour); err != nil {
		return fmt.Errorf("cleanup: sleep: %w", err)
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

	// 2. Confirm the operator hasn't rolled back to the version we're
	// about to delete. GetIndexState returns ActiveVersion=0 when no
	// state row exists (never-indexed article); both 0 and "not equal
	// to the superseded version" mean it's safe to proceed.
	var st actembeddings.GetIndexStateOutput
	var acts *actembeddings.Activities
	if err := workflow.ExecuteActivity(ctx, acts.GetIndexState, &actembeddings.GetIndexStateInput{
		ArticleID: in.SourceID,
	}).Get(ctx, &st); err != nil {
		return fmt.Errorf("cleanup: get state: %w", err)
	}
	if st.ActiveVersion == in.SupersededVersion {
		logger.Info("rollback detected; aborting cleanup",
			"source_id", in.SourceID,
			"superseded_version", in.SupersededVersion)
		return nil
	}

	// 3. Delete.
	if err := workflow.ExecuteActivity(ctx, acts.DeleteOldVectors, &actembeddings.DeleteOldVectorsInput{
		ContentType: in.ContentType,
		SourceID:    in.SourceID,
		Version:     in.SupersededVersion,
	}).Get(ctx, nil); err != nil {
		return fmt.Errorf("cleanup: delete: %w", err)
	}
	logger.Info("cleanup deleted superseded vectors",
		"source_id", in.SourceID,
		"superseded_version", in.SupersededVersion)
	return nil
}
