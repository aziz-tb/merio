# exec-runner

Python execution service for the AI mailbox platform.

Runs untrusted Python code in isolated QEMU microVMs (via [exec-sandbox](https://github.com/dualeai/exec-sandbox))
and exposes a small HTTP API for the Go binary to dispatch tool executions.

## Quick start

```bash
# Install
uv sync
brew install qemu          # macOS, or `apt install qemu-system` on Linux

# Pre-fetch VM images (mandatory for production)
uv run sbx prefetch -q

# Run service
SERVICE_TOKEN=secret123 uv run uvicorn app.main:app --host 0.0.0.0 --port 8090
```

## Endpoints

- `POST /v1/execute` — run a script
- `POST /v1/prewarm` — pre-build snapshot for a package set
- `GET /v1/health/live` — liveness probe
- `GET /v1/health/ready` — readiness probe (gates traffic)
- `GET /metrics` — prometheus metrics

All endpoints except `/v1/health/*` and `/metrics` require `Authorization: Bearer <SERVICE_TOKEN>`.

`POST /v1/execute` returns script-level outcomes as a normal 200 response.
The `result.data` field is any JSON-serializable value emitted by the SDK
(object, array, scalar, or null). Runtime-layer failures, where the service
itself cannot produce a script result, return HTTP 500 with:

```json
{
  "kind": "runtime_error",
  "code": "UNCAUGHT",
  "message": "human-readable error",
  "traceback": "optional stack trace"
}
```

## Design boundaries

- This service runs code; it does NOT know about phases, tools, hooks, agents, or pipelines.
- The Go binary owns business logic and dispatches scripts here.
- The SDK (`import sdk` inside scripts) is a thin wrapper for inputs, outputs, and platform callbacks.
- Idempotency is automatic on writes — script authors do not manage it.

See [`docs/`](./docs) and [`app/sdk_image/sdk/README.md`](./app/sdk_image/sdk/README.md)
for the full plan and SDK reference.

## Configuration

| Variable | Default | Required | Description |
|---|---|---|---|
| `SERVICE_TOKEN` | — | yes | Shared secret with Go binary |
| `SERVICE_PORT` | 8090 | | HTTP port |
| `WARM_POOL_SIZE_PYTHON` | 10 | | Pre-started Python VMs |
| `WARM_POOL_SIZE_JS` | 5 | | Pre-started JavaScript VMs |
| `DEFAULT_MEMORY_MB` | 256 | | Per-VM memory |
| `DEFAULT_TIMEOUT_SECONDS` | 60 | | Per-execution wall-clock cap |
| `MAX_CONCURRENT_EXECUTIONS` | 64 | | Service-level concurrency cap |
| `EXECUTION_QUEUE_SIZE` | 128 | | Admission queue depth |
| `LOG_LEVEL` | INFO | | DEBUG/INFO/WARNING/ERROR |
| `LOG_FORMAT` | json | | json or console |
| `EXEC_SANDBOX_OFFLINE` | 1 | | Set to 1 in prod after `sbx prefetch` |
