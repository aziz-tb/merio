// Package tagclassification implements the Temporal workflow that classifies a
// conversation against the tag catalog.
package tagclassification

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type Input struct {
	ConversationUUID  string
	TriggeredBy       string
	TriggeredByUserID int64
}

type Output struct {
	AIOperationID int64
	AppliedTagIDs []int
	RemovedTagIDs []int
	Status        string
}

type activityInput struct {
	ConversationUUID  string
	TriggeredBy       string
	TriggeredByUserID int64
}

type failureInput struct {
	ConversationUUID string
	ErrorMessage     string
}

func TagClassificationWorkflow(ctx workflow.Context, input Input) (Output, error) {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	})

	var output Output
	err := workflow.ExecuteActivity(activityCtx, "ClassifyTagsActivity", activityInput{
		ConversationUUID:  input.ConversationUUID,
		TriggeredBy:       input.TriggeredBy,
		TriggeredByUserID: input.TriggeredByUserID,
	}).Get(ctx, &output)
	if err != nil {
		notifyCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 1,
			},
		})
		_ = workflow.ExecuteActivity(notifyCtx, "NotifyClassificationFailureActivity", failureInput{
			ConversationUUID: input.ConversationUUID,
			ErrorMessage:     err.Error(),
		}).Get(notifyCtx, nil)
	}
	return output, err
}
