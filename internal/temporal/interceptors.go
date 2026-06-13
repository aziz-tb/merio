package temporal

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/interceptor"

	"github.com/zerodha/logf"
)

// defaultInterceptors returns the default worker interceptor chain (currently
// just activity start/finish logging). Add tracing / metrics here.
func defaultInterceptors(lo *logf.Logger) []interceptor.WorkerInterceptor {
	return []interceptor.WorkerInterceptor{
		&loggingInterceptor{lo: lo},
	}
}

// loggingInterceptor logs every activity invocation, correlated by trace_id
// (see search_attributes.go).
//
// Workflow-side interception is intentionally omitted: workflow code replays
// on history events, so an interceptor would log duplicate lines on every
// replay. Workflows use workflow.GetLogger(ctx) instead.
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
