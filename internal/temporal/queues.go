package temporal

// Task-queue names — the single source of truth that workers register
// against and workflow starters reference. Separate queues so concurrency
// can differ per queue, failures stay isolated, and workers roll out
// independently.
//
// When adding a queue, also add a [temporal.workers.<name>] block in
// config.toml (WorkerConfig.Resolved() defaults apply when absent).
const (
	// QueueEmbeddings handles embedding indexing (KB articles, resolved
	// tickets, bulk reindex). High-volume, short-lived Voyage API activities.
	QueueEmbeddings = "embeddings"

	// QueuePipeline handles per-ticket inbound-pipeline workflows.
	QueuePipeline = "pipeline"

	// QueueAIRuns handles ad-hoc agent runs (the Composer tab's background
	// path and future pipeline-triggered runs).
	QueueAIRuns = "ai-runs"

	// QueueSLAs handles long-running SLA timer workflows: low frequency, very
	// long execution (hours/days), ContinueAsNew to keep history small.
	QueueSLAs = "sla-timers"

	// QueueDefault is the fallback for one-off / experimental workflows not yet
	// promoted to a dedicated queue (smoke tests, one-shot admin actions).
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
