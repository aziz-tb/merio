// Package hello holds the smoke-test workflow used to verify a
// Temporal worker is wired end-to-end.
//
// The workflow follows every convention from
// internal/temporal/CONVENTIONS.md so it doubles as a reference
// example for the first real workflow author.
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
// Deliberately trivial; the goal is to prove the wiring, not to
// demonstrate non-trivial orchestration.
//
// Conventions on display:
//   - explicit ActivityOptions (timeouts + retry policy)
//   - typed pointer-to-struct activity input/output
//   - workflow.GetLogger is used (not logf directly) so log lines
//     come from the SDK's replay-safe logger
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
