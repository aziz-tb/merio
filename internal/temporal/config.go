// Package temporal wraps the Temporal Go SDK with Merio-specific
// init, observability, and shutdown semantics. Workflow + activity code
// lives in internal/workflows/<domain>/ and internal/activities/<domain>/
// respectively; this package is the infrastructure they run on.
//
// See internal/temporal/CONVENTIONS.md for the load-bearing rules every
// workflow + activity author must internalize.
package temporal

import "time"

// Config carries Temporal connection + worker-tuning settings loaded
// from the [temporal] block in config.toml. Defaults are filled in by
// the loader in cmd/init.go — this type only declares shape.
type Config struct {
	// Enabled gates Temporal entirely. When false, the main app boots
	// without a client, and workflow-starting code paths fall back to
	// sync or refuse loudly per the conventions doc.
	Enabled bool `koanf:"enabled"`

	// Address is the Temporal frontend gRPC endpoint. In docker-compose
	// this is "temporal:7233"; for local dev outside compose it's
	// "127.0.0.1:7233".
	Address string `koanf:"address"`

	// Namespace defaults to "merio". Multi-namespace setup (per
	// tenant, per workflow class) is deferred until needed.
	Namespace string `koanf:"namespace"`

	// DialTimeout caps how long client.Dial may block at startup.
	DialTimeout time.Duration `koanf:"dial_timeout"`

	// ShutdownGrace is the time the worker is given to drain
	// in-flight activities after ctx is cancelled before the process
	// exits.
	ShutdownGrace time.Duration `koanf:"shutdown_grace"`

	// Workers carries per-queue tuning. Map key is the task-queue
	// constant from internal/temporal/queues.go.
	Workers map[string]WorkerConfig `koanf:"workers"`
}

// WorkerConfig tunes one queue's worker pool. Sensible per-queue
// defaults exist in [WorkerConfig.Resolved] — callers don't have to
// configure every queue.
type WorkerConfig struct {
	// MaxConcurrentActivities caps the number of activity invocations
	// running on this worker at once. Tune empirically per queue;
	// embedding queues run hot, SLA timer queues run cold.
	MaxConcurrentActivities int `koanf:"max_concurrent_activities"`

	// MaxConcurrentWorkflows caps the number of workflow tasks
	// processed concurrently. Workflow tasks are CPU-light (mostly
	// orchestration); set higher than the activity cap.
	MaxConcurrentWorkflows int `koanf:"max_concurrent_workflows"`
}

// Resolved fills zero-valued fields with sensible defaults. Returns a
// new WorkerConfig — never mutates the receiver, so a config value
// from koanf round-trips cleanly.
func (w WorkerConfig) Resolved() WorkerConfig {
	if w.MaxConcurrentActivities == 0 {
		w.MaxConcurrentActivities = 30
	}
	if w.MaxConcurrentWorkflows == 0 {
		w.MaxConcurrentWorkflows = 100
	}
	return w
}
