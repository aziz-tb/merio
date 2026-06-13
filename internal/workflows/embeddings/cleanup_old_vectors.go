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
// superseded version's vectors, unless the operator has rolled back to
// it in the meantime. workflow.Sleep is used (not a goroutine timer) so
// the wait survives worker restarts. Re-running is a no-op once rows are
// already deleted.
func CleanupOldVectorsWorkflow(ctx workflow.Context, in CleanupOldVectorsInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("CleanupOldVectorsWorkflow scheduled",
		"source_id", in.SourceID,
		"superseded_version", in.SupersededVersion,
		"grace_days", in.GraceDays)

	if in.GraceDays <= 0 {
		in.GraceDays = 7
	}

	// 1. Durable wait.
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

	// 2. Confirm no rollback to the version we're about to delete.
	// GetIndexState returns ActiveVersion=0 for a never-indexed article;
	// anything but an exact match to SupersededVersion is safe to delete.
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
