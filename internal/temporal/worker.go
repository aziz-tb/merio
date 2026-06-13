package temporal

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/sdk/worker"

	"github.com/zerodha/logf"
)

// Worker wraps go.temporal.io/sdk/worker.Worker. One Worker = one task
// queue's pool; the worker binary constructs one per queue.
type Worker struct {
	w         worker.Worker
	client    *Client
	taskQueue string
	lo        *logf.Logger
}

// NewWorker constructs a worker for a single task queue; register workflows
// and activities afterwards via RegisterWorkflow / RegisterActivity. Returns
// an error (rather than panicking) on a nil client or empty queue/logger.
func NewWorker(c *Client, taskQueue string, cfg WorkerConfig, lo *logf.Logger) (*Worker, error) {
	if c == nil {
		return nil, errors.New("temporal: NewWorker requires a non-nil Client (Temporal disabled?)")
	}
	if taskQueue == "" {
		return nil, errors.New("temporal: NewWorker requires a non-empty task queue")
	}
	if lo == nil {
		return nil, errors.New("temporal: NewWorker requires a logger")
	}
	resolved := cfg.Resolved()
	w := worker.New(c.SDK(), taskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     resolved.MaxConcurrentActivities,
		MaxConcurrentWorkflowTaskExecutionSize: resolved.MaxConcurrentWorkflows,
		Interceptors:                           defaultInterceptors(lo),
		// OnFatalError surfaces unrecoverable worker errors (e.g. namespace
		// not found) into our log pipeline.
		OnFatalError: func(err error) {
			lo.Error("temporal worker fatal", "task_queue", taskQueue, "error", err)
		},
	})
	return &Worker{w: w, client: c, taskQueue: taskQueue, lo: lo}, nil
}

// RegisterWorkflow registers a workflow function. Call before Start.
func (w *Worker) RegisterWorkflow(fn any) {
	w.w.RegisterWorkflow(fn)
}

// RegisterActivity registers an activity function (or a struct whose
// exported methods become activities). Call before Start.
func (w *Worker) RegisterActivity(fn any) {
	w.w.RegisterActivity(fn)
}

// Start blocks until ctx is cancelled, then stops accepting new tasks and
// drains in-flight work before returning (graceful shutdown).
func (w *Worker) Start(ctx context.Context) error {
	w.lo.Info("temporal worker starting", "task_queue", w.taskQueue)
	if err := w.w.Run(workerInterruptCh(ctx)); err != nil {
		return fmt.Errorf("temporal worker %s: %w", w.taskQueue, err)
	}
	w.lo.Info("temporal worker stopped", "task_queue", w.taskQueue)
	return nil
}

// workerInterruptCh adapts a context to the channel worker.Run() expects:
// the returned channel closes when ctx is cancelled.
func workerInterruptCh(ctx context.Context) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out
}
