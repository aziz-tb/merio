FROM ghcr.io/astral-sh/uv:python3.12-bookworm

# Enable bookworm-backports to get QEMU 10.x (required for L1 memory snapshots)
RUN echo "deb http://deb.debian.org/debian bookworm-backports main" \
      > /etc/apt/sources.list.d/backports.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends -t bookworm-backports \
      qemu-system-x86 qemu-utils && \
    apt-get install -y --no-install-recommends curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml uv.lock* README.md ./
RUN uv sync --frozen --no-dev

COPY app /app/app

RUN --mount=type=secret,id=github_token \
    if [ -f /run/secrets/github_token ]; then \
      export GITHUB_TOKEN=$(cat /run/secrets/github_token); \
    fi && \
    uv run sbx prefetch -q

ENV EXEC_SANDBOX_OFFLINE=1

EXPOSE 8090
HEALTHCHECK --interval=5s --timeout=3s --start-period=60s --retries=12 \
    CMD curl -f -s http://localhost:8090/v1/health/ready || exit 1

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]