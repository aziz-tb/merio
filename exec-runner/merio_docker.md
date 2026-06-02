The Makefile additions:
makefile# Exec-runner sandbox service (Python microVM execution)
EXEC_RUNNER_DIR := services/exec-runner

.PHONY: exec-runner-deps
exec-runner-deps:
	@echo "→ Installing exec-runner Python dependencies..."
	@cd ${EXEC_RUNNER_DIR} && uv sync

.PHONY: exec-runner-prefetch
exec-runner-prefetch: exec-runner-deps
	@echo "→ Pre-fetching exec-sandbox VM images..."
	@cd ${EXEC_RUNNER_DIR} && uv run sbx prefetch

.PHONY: run-exec-runner
run-exec-runner: exec-runner-deps
	@echo "→ Running exec-runner..."
	@cd ${EXEC_RUNNER_DIR} && SERVICE_TOKEN=$${SERVICE_TOKEN:-devsecret} \
		uv run uvicorn app.main:app --host 0.0.0.0 --port 8090 --reload

.PHONY: test-exec-runner
test-exec-runner: exec-runner-deps
	@echo "→ Running exec-runner tests..."
	@cd ${EXEC_RUNNER_DIR} && uv run pytest

# Update test target to include exec-runner
.PHONY: test
test: test-exec-runner
	@echo "→ Running Go tests..."
	go test -count=1 ./...
	@echo "→ Running frontend tests..."
	cd ${FRONTEND_DIR} && npx pnpm install --frozen-lockfile && npx pnpm test:run
The compose addition:
yaml# docker-compose.yml additions
services:
  app:
    # ... existing config ...
    environment:
      # ... existing env ...
      EXEC_RUNNER_URL: http://exec-runner:8090
      EXEC_RUNNER_TOKEN: ${SERVICE_TOKEN:?SERVICE_TOKEN must be set in .env}
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_started
      exec-runner:
        condition: service_healthy

  exec-runner:
    build: ./services/exec-runner
    container_name: merio_exec_runner
    restart: unless-stopped
    networks:
      - merio
    # KVM device passthrough — required for hardware virtualization
    # On macOS Docker Desktop, this won't work; see docs/exec-runner-dev.md
    devices:
      - /dev/kvm:/dev/kvm
    group_add:
      - "${KVM_GID:-108}"   # match host kvm group GID; varies by distro
    environment:
      SERVICE_TOKEN: ${SERVICE_TOKEN:?SERVICE_TOKEN must be set in .env}
      WARM_POOL_SIZE_PYTHON: ${EXEC_WARM_POOL_PYTHON:-10}
      WARM_POOL_SIZE_JS: ${EXEC_WARM_POOL_JS:-5}
      DEFAULT_MEMORY_MB: ${EXEC_VM_MEMORY:-256}
      EXEC_SANDBOX_OFFLINE: "1"
      LOG_FORMAT: json
    volumes:
      # Cache exec-sandbox VM images across container restarts
      - exec-sandbox-cache:/root/.cache/exec-sandbox
    deploy:
      resources:
        limits:
          memory: 12G
        reservations:
          memory: 4G
    healthcheck:
      test: ["CMD", "curl", "-f", "-s", "http://localhost:8090/v1/health/ready"]
      interval: 5s
      timeout: 3s
      retries: 12
      start_period: 60s
    # Expose internally only — Go binary reaches it via Docker network
    expose:
      - "8090"

volumes:
  postgres-data:
  redis-data:
  exec-sandbox-cache:
