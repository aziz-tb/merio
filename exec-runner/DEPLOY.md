# Deployment guide

How to run `exec-runner` in production.

## Prerequisites

- Linux host with KVM support (`/dev/kvm` exists), or macOS (HVF) for development
- Python 3.12+
- QEMU 8.0+ installed (`apt install qemu-system` on Debian/Ubuntu)
- Sufficient memory: ~4–6 GB for the warm pool plus headroom — see [Resource sizing](#resource-sizing)

## Recommended deployment shape

Same host as the Go binary, supervised by systemd. The Go binary talks to
the sandbox over `http://127.0.0.1:8090`. Sub-millisecond network latency.

```
┌─────────────────────────────────┐
│  Host                            │
│  ┌──────────┐    ┌────────────┐ │
│  │   Go     │ ─▶ │ exec-runner│ │
│  │  binary  │    │  (Python)  │ │
│  └──────────┘    └────────────┘ │
│       │                │         │
│       └─────┬──────────┘         │
│             │                    │
│       Postgres + Redis           │
└─────────────────────────────────┘
```

## Linux production setup (systemd)

### 1. Create the qemu-vm system user

Required for production hardening. Without it, exec-sandbox runs QEMU as
the service user (less isolated).

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin qemu-vm
sudo usermod -aG kvm qemu-vm

# Service user (e.g. 'sandbox') needs membership in qemu-vm group
# to access QEMU's Unix sockets (0660 permissions).
sudo useradd --system --create-home --home-dir /var/lib/exec-runner sandbox
sudo usermod -aG qemu-vm sandbox
```

### 2. Install the service

```bash
# Build artifact location: /opt/exec-runner
sudo mkdir -p /opt/exec-runner
sudo chown sandbox:sandbox /opt/exec-runner

# Drop in the code
sudo -u sandbox cp -r app pyproject.toml /opt/exec-runner/

# Install Python deps
sudo -u sandbox bash -c 'cd /opt/exec-runner && uv sync --frozen'

# Pre-fetch VM images once (mandatory — never rely on runtime download)
sudo -u sandbox bash -c 'cd /opt/exec-runner && uv run sbx prefetch -q'
```

### 3. systemd unit

Save as `/etc/systemd/system/exec-runner.service`:

```ini
[Unit]
Description=AI mailbox — Python execution sandbox
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=sandbox
Group=sandbox
WorkingDirectory=/opt/exec-runner

# Environment — read from /etc/exec-runner.env
EnvironmentFile=/etc/exec-runner.env
Environment=EXEC_SANDBOX_OFFLINE=1

# Startup
ExecStart=/usr/bin/uv run uvicorn app.main:app --host 0.0.0.0 --port 8090 --workers 1

# Resource limits — prevent runaway from taking down the host
MemoryMax=12G
CPUQuota=400%
TasksMax=2048

# Restart on failure with backoff
Restart=on-failure
RestartSec=10

# Reasonable shutdown timeout — let in-flight executions complete
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target
```

Save environment to `/etc/exec-runner.env` (mode 0640, owned by root):

```env
SERVICE_TOKEN=<long random string shared with the Go binary>
SERVICE_PORT=8090
WARM_POOL_SIZE_PYTHON=10
WARM_POOL_SIZE_JS=5
DEFAULT_MEMORY_MB=256
DEFAULT_TIMEOUT_SECONDS=60
MAX_CONCURRENT_EXECUTIONS=64
EXECUTION_QUEUE_SIZE=128
LOG_LEVEL=INFO
LOG_FORMAT=json
```

### 4. Enable and start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now exec-runner.service
sudo systemctl status exec-runner

# Watch logs
sudo journalctl -u exec-runner -f
```

### 5. Coordinate Go binary startup

The Go binary's systemd unit should declare:

```ini
[Unit]
After=exec-runner.service
Wants=exec-runner.service
```

And on startup, the Go binary should poll `http://127.0.0.1:8090/v1/health/ready`
with backoff before accepting traffic. See the foundation strategy doc for
the polling shape (1s, 2s, 4s, max 30s, total cap 5 minutes).

If readiness doesn't arrive within 5 min, the Go binary starts in **degraded
mode** — inbox features work, AI features are disabled, an operator banner
is shown.

## Docker / Kubernetes

### Dockerfile

```dockerfile
FROM ghcr.io/astral-sh/uv:python3.12-bookworm

# Install QEMU
RUN apt-get update && apt-get install -y qemu-system && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python dependencies
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen

# Pre-fetch VM images at build time so first start is fast and offline-safe
RUN uv run sbx prefetch -q
ENV EXEC_SANDBOX_OFFLINE=1

# Copy application code
COPY app/ /app/app/

EXPOSE 8090 9090
CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090", "--workers", "1"]
```

### Kubernetes Pod (sidecar pattern)

Run the sandbox alongside the Go binary in the same Pod. Sandbox has its
own readiness probe; the Pod doesn't accept traffic until both are ready.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mailbox-app
spec:
  containers:
    - name: go-binary
      image: ghcr.io/your-org/mailbox:latest
      env:
        - name: SANDBOX_URL
          value: "http://localhost:8090"
        - name: SERVICE_TOKEN
          valueFrom: { secretKeyRef: { name: sandbox-token, key: token } }
      readinessProbe:
        httpGet: { path: /healthz, port: 8080 }

    - name: exec-runner
      image: ghcr.io/your-org/exec-runner:latest
      ports:
        - containerPort: 8090
        - containerPort: 9090
      env:
        - name: SERVICE_TOKEN
          valueFrom: { secretKeyRef: { name: sandbox-token, key: token } }
        - name: WARM_POOL_SIZE_PYTHON
          value: "10"
        - name: WARM_POOL_SIZE_JS
          value: "5"
      resources:
        requests: { memory: 6Gi, cpu: 2 }
        limits:   { memory: 12Gi, cpu: 4 }
      securityContext:
        # KVM device access required
        privileged: true
      livenessProbe:
        httpGet: { path: /v1/health/live, port: 8090 }
        initialDelaySeconds: 5
        periodSeconds: 10
        failureThreshold: 3
      readinessProbe:
        httpGet: { path: /v1/health/ready, port: 8090 }
        initialDelaySeconds: 30
        periodSeconds: 5
        failureThreshold: 6
        successThreshold: 1
```

> **Privileged container.** KVM requires `/dev/kvm` access and additional
> kernel privileges. Cluster operators may need to allow privileged Pods
> for this namespace, or use a runtime that exposes KVM (firecracker-pod,
> kata-containers, etc.).

## Development setup (macOS / Linux dev)

```bash
# One-time
brew install qemu                              # macOS
# or: sudo apt install qemu-system             # Linux
uv sync
uv run sbx prefetch                             # ~3 min first time

# Run
SERVICE_TOKEN=devsecret uv run uvicorn app.main:app --reload --port 8090
```

Without `qemu-vm` user, exec-sandbox runs QEMU as your user — fine for dev.

## Resource sizing

For default config (10 Python + 5 JS warm pool, 256 MB per VM):

| Component | Memory |
|---|---|
| Warm pool (15 VMs × 256 MB) | 3.84 GB |
| Active executions (variable) | up to ~4 GB at peak |
| Snapshot caches (L1 + L2) | ~1 GB |
| Python service runtime | ~250 MB |
| exec-sandbox overhead | ~500 MB |
| **Subtotal** | **~10 GB** |
| OS + page cache headroom | +2-4 GB |
| **Recommended host RAM** | **12–16 GB** |

CPU: 4–8 cores. exec-sandbox's default overcommit (CPU 2x, MEM 5x) lets
you run more concurrent VMs than physical cores — don't lower these unless
load tests show pressure.

If running co-located with the Go binary, add ~1–2 GB for Go process,
Postgres connection pool, Redis client.

## Operations

### Health checks

- `/v1/health/live` — liveness probe (load balancer / k8s)
- `/v1/health/ready` — readiness probe — gates incoming traffic during warmup
- Returns 503 with `status: "starting"` until pool is at 50% target

### Logs

JSON to stdout. Capture with journald (systemd) or the cluster's log agent
(k8s). Every log line includes `trace_id` correlated with the Go binary's logs.

**Important**: the service does NOT log script stdout — that may contain
PII. Script output is returned in the response and stored by the Go side
according to its retention policy.

### Metrics

Prometheus scrape at `:9090/metrics`. Suggested alerts:

- `rate(exec_sandbox_executions_total{result_kind="error"}[5m]) > 0.05` → script error rate > 5%
- `rate(exec_sandbox_vm_errors_total[1m]) > 0.01` → sandbox errors > 1%
- `exec_sandbox_warm_pool_size{language="python"} < target * 0.5` for 5m → pool starvation
- `exec_sandbox_admission_queue_depth > 50` → load shedding imminent

### Restarts

Cached snapshots survive restart. A warm restart re-populates the pool in
~5–10s (vs. 30–60s on a cold cache). systemd's `Restart=on-failure` with
10s backoff is appropriate.

### Key rotation

`SERVICE_TOKEN` is symmetric. To rotate without downtime:

1. Roll a new token; configure the Go binary to send EITHER (old OR new) for a transition window.
2. Restart sandbox with the new token.
3. Update Go binary to send only new token.
4. Retire old token.

Without graceful rotation, plan a brief maintenance window: stop Go traffic, restart sandbox, restart Go binary.

## Troubleshooting

### "Sandbox scheduler not initialized"

The lifespan startup raised before `init_sandbox` completed. Check journald
for the original error — most often a missing QEMU binary, KVM access
denied, or VM image download failure (set `EXEC_SANDBOX_OFFLINE=1` only
after `sbx prefetch` succeeded).

### `/v1/health/ready` returns 503 forever

Pool isn't reaching 50% of target. Causes:

- Asset download failed (check service logs for `AssetError`)
- Insufficient memory to spawn warm VMs (check host RSS, lower warm pool)
- KVM permission denied (add service user to `kvm` group)

### High `vm_transient` rate

Often host CPU contention or stale exec-sandbox version. Check:

```bash
# Are KVM modules loaded?
lsmod | grep kvm

# CPU pressure?
cat /proc/pressure/cpu

# QEMU version?
qemu-system-x86_64 --version
```

### Scripts timing out at unexpected points

`DEFAULT_TIMEOUT_SECONDS=60` is the floor; per-tool overrides come from
the Go side. If many tools need >60s, raise the default — but understand
that long executions block warm-pool slots.
