"""
FastAPI application — entry point.

Lifespan:
  startup → load config, set up logging, init exec-sandbox scheduler,
            init execution gate, kick off background prewarming
  shutdown → tear down scheduler

Routes mounted:
  /v1/execute              POST  — run a script
  /v1/prewarm              POST  — pre-build a snapshot
  /v1/health/live          GET   — liveness probe
  /v1/health/ready         GET   — readiness probe
  /metrics                 GET   — prometheus metrics
"""
from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from app.config import Config
from app.observability.logging import get_logger, setup_logging
from app.routes import execute as execute_route
from app.routes import health as health_route
from app.routes import prewarm as prewarm_route
from app.runner.execute import init_gate
from app.runner.scheduler import init_sandbox, shutdown_sandbox


@asynccontextmanager
async def lifespan(app: FastAPI):
    # ─── Load config first so logging picks up the configured level ──
    cfg = Config.from_env()
    setup_logging(level=cfg.log_level, log_format=cfg.log_format)
    logger = get_logger("app.main")

    logger.info(
        "service.startup",
        port=cfg.service_port,
        warm_pool_python=cfg.warm_pool_size_python,
        warm_pool_js=cfg.warm_pool_size_js,
        max_concurrent=cfg.max_concurrent_executions,
    )

    # Make config available to dependencies (auth, etc.)
    app.state.config = cfg

    # Initialize the admission gate before the sandbox so requests that race
    # the prewarm don't crash on a missing gate.
    init_gate(cfg.max_concurrent_executions, cfg.execution_queue_size)

    # Initialize the sandbox — this is the slow startup step.
    # init_sandbox kicks off prewarming as a background task.
    try:
        await init_sandbox(cfg)
    except Exception:
        logger.exception("service.startup.sandbox_init_failed")
        # Re-raise so the process exits — we cannot serve without the sandbox.
        raise

    logger.info("service.ready")

    yield

    # ─── Shutdown ───
    logger.info("service.shutdown.start")
    await shutdown_sandbox()
    logger.info("service.shutdown.done")


app = FastAPI(
    title="exec-runner",
    description="Python execution service for the AI mailbox platform",
    version="0.1.0",
    lifespan=lifespan,
)

# Routes
app.include_router(execute_route.router)
app.include_router(prewarm_route.router)
app.include_router(health_route.router)


@app.get("/metrics", include_in_schema=False)
async def metrics() -> Response:
    """Prometheus scrape endpoint. No auth — bind to internal interface only."""
    return Response(
        content=generate_latest(),
        media_type=CONTENT_TYPE_LATEST,
    )
