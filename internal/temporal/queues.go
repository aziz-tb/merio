package temporal

// Task-queue names. Each queue gets its own worker pool. Constants
// here are the single source of truth — workers register against
// these names, workflow starters reference them.
//
// Why separate queues:
//   - Worker concurrency can differ per queue (embedding workers run
//     hot — many API-bound activities; SLA workers run cold — long
//     idle stretches between timer fires).
//   - Failure isolation: if the embedding API is down, the pipeline
//     queue keeps moving.
//   - Independent rollout: a new embedding-worker binary can deploy
//     without restarting pipeline workers.
//   - Future: different namespaces / retention policies if needed.
//
// When adding a queue, also add a worker-config block in
// [temporal.workers.<name>] in config.toml (defaults via
// WorkerConfig.Resolved() apply when the block is absent).
const (
	// QueueEmbeddings handles embedding indexing — KB articles,
	// resolved tickets, bulk reindex jobs. High-volume, short-lived
	// activities (Voyage API calls). F12 builds the workflows; F1
	// only ships the queue name.
	QueueEmbeddings = "embeddings"

	// QueuePipeline handles inbound-pipeline workflows (when they
	// ship). Per-ticket execution; activities span the full 11-stage
	// processing.
	QueuePipeline = "pipeline"

	// QueueAIRuns handles ad-hoc agent runs — the Composer tab's
	// background path and future pipeline-triggered runs. Currently
	// the runner is in-process; it moves here when L2/L3 autonomy
	// needs durable execution.
	QueueAIRuns = "ai-runs"

	// QueueSLAs handles long-running SLA timer workflows (when they
	// ship). Low call frequency, very long execution time (hours /
	// days); workflows use ContinueAsNew to keep history small.
	QueueSLAs = "sla-timers"

	// QueueDefault is the fallback for one-off / experimental
	// workflows not yet promoted to a dedicated queue. Useful for
	// hello-world smoke tests and one-shot admin actions.
	QueueDefault = "default"
)

// AllQueues lists every named queue. Used by the worker binary to
// spin up one worker per queue at boot.
var AllQueues = []string{
	QueueEmbeddings,
	QueuePipeline,
	QueueAIRuns,
	QueueSLAs,
	QueueDefault,
}
