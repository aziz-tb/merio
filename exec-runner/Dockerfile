FROM ghcr.io/astral-sh/uv:python3.12-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-system-x86 qemu-utils curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY pyproject.toml uv.lock* README.md ./
RUN uv sync --frozen --no-dev

COPY app /app/app

# Allow GITHUB_TOKEN to be passed in for sbx prefetch rate limits
ARG GITHUB_TOKEN
ENV GITHUB_TOKEN=${GITHUB_TOKEN}

RUN uv run sbx prefetch -q

ENV EXEC_SANDBOX_OFFLINE=1

EXPOSE 8090
HEALTHCHECK --interval=5s --timeout=3s --start-period=60s --retries=12 \
    CMD curl -f -s http://localhost:8090/v1/health/ready || exit 1

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
