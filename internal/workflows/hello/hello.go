// Package hello holds the smoke-test workflow used to verify a Temporal
// worker is wired end-to-end. It follows internal/temporal/CONVENTIONS.md.
package hello

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	hellact "github.com/aziz-tb/merio/internal/activities/hello"
)

// SayHelloInput is the workflow input.
type SayHelloInput struct {
	Name string
}

// SayHelloOutput is the workflow output.
type SayHelloOutput struct {
	Greeting string
}

// SayHelloWorkflow invokes one activity and returns its result.
func SayHelloWorkflow(ctx workflow.Context, in SayHelloInput) (SayHelloOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("SayHelloWorkflow started", "name", in.Name)

	opts := workflow.ActivityOptions{
		StartToCloseTimeout:    10 * time.Second,
		ScheduleToCloseTimeout: 1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			MaximumInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)

	var (
		acts *hellact.Activities // typed for the symbol lookup; nil at workflow author time
		out  hellact.SayHelloOutput
	)
	err := workflow.ExecuteActivity(ctx, acts.SayHello, &hellact.SayHelloInput{
		Name: in.Name,
	}).Get(ctx, &out)
	if err != nil {
		return SayHelloOutput{}, err
	}

	logger.Info("SayHelloWorkflow finished", "greeting", out.Greeting)
	return SayHelloOutput{Greeting: out.Greeting}, nil
}
