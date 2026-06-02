# Try to get the commit hash from 1) git 2) the VERSION file 3) fallback.
LAST_COMMIT := $(or $(shell git rev-parse --short HEAD 2> /dev/null),$(shell head -n 1 VERSION | grep -Eo "^[a-z0-9]+$$"), "")

# Try to get the semver from 1) git 2) the VERSION file 3) fallback.
VERSION := $(or $(MERIO_VERSION),$(shell git describe --tags --abbrev=0 2> /dev/null),$(shell grep -Eo 'tag: v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?' VERSION | head -1 | sed 's/^tag: //'),"v0.0.0")

BUILDSTR := ${VERSION} (\#${LAST_COMMIT} $(shell date -u +"%Y-%m-%dT%H:%M:%S%z"))

# Binary names and paths
BIN := merio
WORKER_BIN := temporal-worker
FRONTEND_DIR := frontend
FRONTEND_DIST := ${FRONTEND_DIR}/dist
STATIC := ${FRONTEND_DIST} i18n schema.sql static
GOPATH ?= $(HOME)/go
STUFFBIN ?= $(GOPATH)/bin/stuffbin

# The default target to run when `make` is executed.
.DEFAULT_GOAL := build 

# Install stuffbin if it doesn't exist.
$(STUFFBIN):
	@echo "→ Installing stuffbin..."
	@go install github.com/knadh/stuffbin/...

# Install dependencies for both backend and frontend.
.PHONY: install-deps
install-deps: $(STUFFBIN)
	@echo "→ Installing frontend dependencies..."
	@cd ${FRONTEND_DIR} && pnpm install

# Build the frontend for production (both apps).
.PHONY: frontend-build
frontend-build: install-deps
	@echo "→ Building frontend for production - main app & widget..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm build:main
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm build:widget

# Build only the main frontend app.
.PHONY: frontend-build-main
frontend-build-main: install-deps
	@echo "→ Building main frontend app for production..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm build:main

# Build only the widget frontend app.
.PHONY: frontend-build-widget
frontend-build-widget: install-deps
	@echo "→ Building widget frontend app for production..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm build:widget

# Run the Go backend server in development mode.
.PHONY: run-backend
run-backend:
	@echo "→ Running backend..."
	CGO_ENABLED=0 go run -ldflags="-s -w -X 'main.buildString=${BUILDSTR}' -X 'main.versionString=${VERSION}' -X 'github.com/aziz-tb/merio/internal/version.Version=${VERSION}' -X 'main.frontendDir=frontend/dist'" ./cmd/

# Run the JS frontend server in development mode (main app only).
.PHONY: run-frontend
run-frontend:
	@echo "→ Installing frontend dependencies (if not already installed)..."
	@cd ${FRONTEND_DIR} && pnpm install
	@echo "→ Running main frontend app..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm dev:main

# Run the main frontend app in development mode.
.PHONY: run-frontend-main
run-frontend-main:
	@echo "→ Installing frontend dependencies (if not already installed)..."
	@cd ${FRONTEND_DIR} && pnpm install
	@echo "→ Running main frontend app..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm dev:main

# Run the widget frontend app in development mode.
.PHONY: run-frontend-widget
run-frontend-widget:
	@echo "→ Installing frontend dependencies (if not already installed)..."
	@cd ${FRONTEND_DIR} && pnpm install
	@echo "→ Running widget frontend app..."
	@export VITE_APP_VERSION="${VERSION}" && cd ${FRONTEND_DIR} && pnpm dev:widget

# Build the backend binary.
.PHONY: build-backend
build-backend: $(STUFFBIN)
	@echo "→ Building backend..."
	@CGO_ENABLED=0 go build -a \
		-ldflags="-X 'main.buildString=${BUILDSTR}' -X 'main.versionString=${VERSION}' -X 'github.com/aziz-tb/merio/internal/version.Version=${VERSION}' -s -w" \
		-o ${BIN} cmd/*.go

# Build the Temporal worker binary (separate from the main app).
# Workers can be deployed independently of the HTTP API and scale on a
# different cadence. See docs/temporal.md.
.PHONY: build-temporal-worker
build-temporal-worker:
	@echo "→ Building Temporal worker..."
	@CGO_ENABLED=0 go build \
		-ldflags="-s -w" \
		-o ${WORKER_BIN} ./cmd/temporal_worker

# Run the Temporal worker locally against the local Temporal server.
.PHONY: run-temporal-worker
run-temporal-worker:
	@echo "→ Running Temporal worker..."
	@go run ./cmd/temporal_worker --config config.toml

# Main build target: builds both frontend and backend, then stuffs static assets into the binary.
.PHONY: build
build: frontend-build build-backend stuff
	@echo "→ Build successful. Current version: $(VERSION)"

# Stuff static assets into the binary using stuffbin.
.PHONY: stuff
stuff: $(STUFFBIN)
	@echo "→ Stuffing static assets into binary..."
	@$(STUFFBIN) -a stuff -in ${BIN} -out ${BIN} ${STATIC}

# Build the application in demo mode.
.PHONY: demo-build
demo-build:
	@echo "→ Building in demo mode..."
	@export VITE_DEMO_BUILD="true" && $(MAKE) build

# Run tests.
.PHONY: test
test:
	@echo "→ Running Go tests..."
	go test -count=1 ./...
	@echo "→ Running frontend tests..."
	cd ${FRONTEND_DIR} && npx pnpm install --frozen-lockfile && npx pnpm test:run

# ─── Deployment ──────────────────────────────────────────────────────
#
# These targets always re-export VERSION and GIT_COMMIT from git so the
# rebuilt image carries the exact commit it was built from. After deploy,
# `make deploy-verify` greps the container logs to show which version is
# actually running — the answer to "did my rebuild actually take?"

DEPLOY_VERSION := $(or $(MERIO_VERSION),$(shell git describe --tags --always --dirty 2> /dev/null),dev)
DEPLOY_COMMIT := $(or $(LAST_COMMIT),$(shell git rev-parse --short HEAD 2> /dev/null),unknown)
COMPOSE_BUILD_ENV := VERSION=$(DEPLOY_VERSION) GIT_COMMIT=$(DEPLOY_COMMIT)

# Full deploy: rebuild + recreate both app and worker.
.PHONY: deploy
deploy:
	@echo "→ Deploying app + worker (version=$(DEPLOY_VERSION), commit=$(DEPLOY_COMMIT))"
	@$(COMPOSE_BUILD_ENV) docker compose build --pull app temporal-worker
	@docker compose up -d --force-recreate app temporal-worker
	@sleep 3
	@$(MAKE) deploy-verify

# Worker-only deploy. Use this when iterating on workflow/activity code.
.PHONY: deploy-worker
deploy-worker:
	@echo "→ Deploying worker (version=$(DEPLOY_VERSION), commit=$(DEPLOY_COMMIT))"
	@$(COMPOSE_BUILD_ENV) docker compose build temporal-worker
	@docker compose up -d --force-recreate temporal-worker
	@sleep 2
	@docker compose logs --tail=20 temporal-worker

# App-only deploy. Use this for HTTP API / frontend changes.
.PHONY: deploy-app
deploy-app:
	@echo "→ Deploying app (version=$(DEPLOY_VERSION), commit=$(DEPLOY_COMMIT))"
	@$(COMPOSE_BUILD_ENV) docker compose build app
	@docker compose up -d --force-recreate app
	@sleep 3
	@docker compose logs --tail=20 app

# Print versions currently running. Greps the startup log lines, which
# both binaries emit as their very first line of output.
.PHONY: deploy-verify
deploy-verify:
	@echo ""
	@echo "→ Deployed versions:"
	@APP_VER=$$(docker compose logs app 2>/dev/null | grep -m1 -oE "version=[^ ]+" | tail -1); \
	 WORKER_VER=$$(docker compose logs temporal-worker 2>/dev/null | grep -m1 -oE "version=[^ ]+" | tail -1); \
	 echo "  app:    $${APP_VER:-(no version log found; check 'docker compose logs app | head -5')}"; \
	 echo "  worker: $${WORKER_VER:-(no version log found; check 'docker compose logs temporal-worker | head -5')}"; \
	 echo "  source: version=$(DEPLOY_VERSION) commit=$(DEPLOY_COMMIT)"

# Start all services without rebuilding (useful after a host reboot).
.PHONY: up
up:
	@docker compose up -d
	@docker compose ps

# Stop everything cleanly.
.PHONY: down
down:
	@docker compose down

# Tail logs from app + worker (most useful day-to-day).
.PHONY: logs
logs:
	@docker compose logs -f app temporal-worker

# Production deploy targets - run on the VPS only.
.PHONY: prod-up
prod-up:
	@docker compose -f docker-compose.prod.yml up -d
	@docker compose -f docker-compose.prod.yml ps

.PHONY: prod-down
prod-down:
	@docker compose -f docker-compose.prod.yml down

.PHONY: prod-logs
prod-logs:
	@docker compose -f docker-compose.prod.yml logs -f app temporal-worker

.PHONY: prod-pull
prod-pull:
	@docker compose -f docker-compose.prod.yml pull

.PHONY: prod-restart
prod-restart: prod-pull
	@docker compose -f docker-compose.prod.yml up -d --remove-orphans
	@sleep 5
	@docker compose -f docker-compose.prod.yml ps
