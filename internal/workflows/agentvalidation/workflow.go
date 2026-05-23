package agentvalidation

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
	OperationID      int64
	MatchingAgentIDs []int
	Status           string
}

type activityInput struct {
	ConversationUUID  string
	TriggeredBy       string
	TriggeredByUserID int64
}

func AgentValidationWorkflow(ctx workflow.Context, input Input) (Output, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
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
	err := workflow.ExecuteActivity(ctx, "ValidateAgentsActivity", activityInput{
		ConversationUUID:  input.ConversationUUID,
		TriggeredBy:       input.TriggeredBy,
		TriggeredByUserID: input.TriggeredByUserID,
	}).Get(ctx, &output)
	return output, err
}
