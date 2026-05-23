package temporal

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/interceptor"

	"github.com/zerodha/logf"
)

// defaultInterceptors returns the project-default worker interceptor
// chain. Currently one interceptor: structured logging on every
// activity start/finish. Add tracing / metrics interceptors here as
// they land.
func defaultInterceptors(lo *logf.Logger) []interceptor.WorkerInterceptor {
	return []interceptor.WorkerInterceptor{
		&loggingInterceptor{lo: lo},
	}
}

// loggingInterceptor emits log lines on every activity invocation so
// Temporal execution shows up alongside the rest of the app's logs
// (correlated by trace_id via search attributes — see
// internal/temporal/search_attributes.go).
//
// Workflow-side interception is intentionally NOT done here: workflow
// code is replayed on history events, so logging from inside an
// interceptor would emit duplicate lines on every replay. Workflows
// use workflow.GetLogger(ctx) directly for that reason.
type loggingInterceptor struct {
	interceptor.WorkerInterceptorBase
	lo *logf.Logger
}

func (l *loggingInterceptor) InterceptActivity(
	_ context.Context,
	next interceptor.ActivityInboundInterceptor,
) interceptor.ActivityInboundInterceptor {
	return &activityLogger{
		ActivityInboundInterceptorBase: interceptor.ActivityInboundInterceptorBase{Next: next},
		lo:                             l.lo,
	}
}

type activityLogger struct {
	interceptor.ActivityInboundInterceptorBase
	lo *logf.Logger
}

func (a *activityLogger) ExecuteActivity(
	ctx context.Context,
	in *interceptor.ExecuteActivityInput,
) (any, error) {
	info := activity.GetInfo(ctx)
	a.lo.Info("activity.start",
		"activity_type", info.ActivityType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"run_id", info.WorkflowExecution.RunID,
		"attempt", info.Attempt,
		"task_queue", info.TaskQueue,
	)

	started := time.Now()
	result, err := a.ActivityInboundInterceptorBase.ExecuteActivity(ctx, in)
	durMs := time.Since(started).Milliseconds()

	if err != nil {
		a.lo.Warn("activity.finish",
			"activity_type", info.ActivityType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", durMs,
			"success", false,
			"error", err.Error(),
		)
	} else {
		a.lo.Info("activity.finish",
			"activity_type", info.ActivityType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", durMs,
			"success", true,
		)
	}
	return result, err
}
