"""
Prometheus metrics for operator dashboards and alerts.

Minimum set for future implementations. Exposed at /metrics by main.py.
"""
from __future__ import annotations

from prometheus_client import Counter, Gauge, Histogram

# Per-execution counters
executions_total = Counter(
    "exec_sandbox_executions_total",
    "Total executions processed",
    ["language", "exit_code", "result_kind"],
)

execution_duration_ms = Histogram(
    "exec_sandbox_execution_duration_ms",
    "End-to-end execution duration in milliseconds",
    ["language"],
    buckets=(50, 100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000, 120000),
)

# Warm pool state
warm_pool_size = Gauge(
    "exec_sandbox_warm_pool_size",
    "Current number of warm VMs",
    ["language"],
)

warm_pool_target = Gauge(
    "exec_sandbox_warm_pool_target",
    "Target number of warm VMs",
    ["language"],
)

# Concurrency
concurrent_executions = Gauge(
    "exec_sandbox_concurrent_executions",
    "Current in-flight executions",
)

admission_queue_depth = Gauge(
    "exec_sandbox_admission_queue_depth",
    "Number of requests waiting for an execution slot",
)

# Errors
vm_errors_total = Counter(
    "exec_sandbox_vm_errors_total",
    "Sandbox-level errors (not script errors)",
    ["error_type"],
)

# Prewarm
prewarm_total = Counter(
    "exec_sandbox_prewarm_total",
    "Prewarm requests processed",
    ["language", "outcome"],
)
