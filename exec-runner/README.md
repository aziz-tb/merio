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

## Docker on Linux/KVM

The production compose stack runs exec-runner as `merio_exec_runner` on the
same Docker network as `merio_app`. The Go containers should use:

```env
MERIO_EXEC__SANDBOX_URL=http://exec-runner:8090
MERIO_EXEC__PLATFORM_BASE_URL=http://merio_app:9000
```

The container needs `/dev/kvm` and the host `kvm` group GID:

```bash
KVM_GID=$(getent group kvm | cut -d: -f3)
docker compose -f docker-compose.prod.yml pull exec-runner
docker compose -f docker-compose.prod.yml up -d exec-runner
```

The production image is published to
`ghcr.io/aziz-tb/merio-exec-runner` on tagged releases. The VPS does not need
the `exec-runner/` source tree.

macOS Docker Desktop does not expose `/dev/kvm`; keep running the service on
the host with `uv` for Mac development.

Linux production images require QEMU 9+. QEMU 7.x/8.x can fail L1 memory
snapshot saves; the production image installs QEMU from `bookworm-backports`.

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
- Write idempotency is **planned but not yet implemented**: the SDK sends an `Idempotency-Key` header, but the Go platform does not yet honor it, so a retried write can double-execute.

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
| `CPU_OVERCOMMIT_RATIO` | 2.0 | | exec-sandbox CPU overcommit ratio; defaults are fine for most setups |
| `MEMORY_OVERCOMMIT_RATIO` | 5.0 | | exec-sandbox memory overcommit ratio; tune when scaling concurrency |
| `MAX_CONCURRENT_EXECUTIONS` | 64 | | Service-level concurrency cap |
| `EXECUTION_QUEUE_SIZE` | 128 | | Admission queue depth |
| `LOG_LEVEL` | INFO | | DEBUG/INFO/WARNING/ERROR |
| `LOG_FORMAT` | json | | json or console |
| `EXEC_SANDBOX_OFFLINE` | 1 | | Set to 1 in prod after `sbx prefetch` |
