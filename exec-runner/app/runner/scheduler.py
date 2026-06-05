# exec-runner/app/runner/scheduler.py
"""
Sandbox scheduler lifecycle management.

Singleton exec-sandbox Scheduler shared across all executions. Initialized at
app startup, shut down on app shutdown. Pre-warming runs in background after
init so /health/live returns 200 immediately while /health/ready stays 503
until the warm pool reaches target.
"""
from __future__ import annotations

import asyncio
from typing import Optional

from exec_sandbox import Scheduler, SchedulerConfig

from app.config import Config
from app.observability.logging import get_logger
from app.observability.metrics import warm_pool_size, warm_pool_target

logger = get_logger(__name__)

_scheduler: Optional[Scheduler] = None
_initialized: bool = False
_prewarm_task: Optional[asyncio.Task] = None


def get_scheduler() -> Scheduler:
    """Return the shared scheduler. Raises if not yet initialized."""
    if _scheduler is None:
        raise RuntimeError("Sandbox scheduler not initialized — call init_sandbox() first")
    return _scheduler


def is_ready() -> bool:
    """True once init_sandbox has completed."""
    return _initialized


async def init_sandbox(cfg: Config) -> None:
    """
    Initialize the singleton Scheduler. Call once during app startup.

    Pre-warming is launched as a background task — readiness is gated separately
    via /health/ready, which checks the actual warm pool population.
    """
    global _scheduler, _initialized, _prewarm_task

    if _initialized:
        logger.warning("sandbox.init.already_initialized")
        return

    # We pass warm_pool_size=N which exec-sandbox interprets as N per language.
    # We pick max(python, js) since exec-sandbox uses one number for all.
    # If the two values diverge significantly, this is a known limitation —
    # we'd need to upstream a per-language warm pool to exec-sandbox.
    pool_size = max(cfg.warm_pool_size_python, cfg.warm_pool_size_js)

    sandbox_cfg = SchedulerConfig(
        warm_pool_size=pool_size,
        default_memory_mb=cfg.default_memory_mb,
        default_timeout_seconds=cfg.default_timeout_seconds,
        enable_package_validation=cfg.enable_package_validation,
        cpu_overcommit_ratio=cfg.cpu_overcommit_ratio,
        memory_overcommit_ratio=cfg.memory_overcommit_ratio,
    )

    _scheduler = Scheduler(sandbox_cfg)
    await _scheduler.__aenter__()
    _initialized = True

    # Set target gauges
    warm_pool_target.labels(language="python").set(cfg.warm_pool_size_python)
    warm_pool_target.labels(language="javascript").set(cfg.warm_pool_size_js)

    logger.info(
        "sandbox.init.ok",
        warm_pool_python=cfg.warm_pool_size_python,
        warm_pool_js=cfg.warm_pool_size_js,
        memory_mb=cfg.default_memory_mb,
        timeout_seconds=cfg.default_timeout_seconds,
        cpu_overcommit_ratio=cfg.cpu_overcommit_ratio,
        memory_overcommit_ratio=cfg.memory_overcommit_ratio,
    )

    if cfg.prewarm_on_startup:
        # Run prewarm in background so app startup isn't blocked
        from app.runner.prewarm import prewarm_common_snapshots

        _prewarm_task = asyncio.create_task(prewarm_common_snapshots())


async def shutdown_sandbox() -> None:
    """Tear down the scheduler. Call at app shutdown."""
    global _scheduler, _initialized, _prewarm_task

    if _prewarm_task is not None and not _prewarm_task.done():
        _prewarm_task.cancel()
        try:
            await _prewarm_task
        except asyncio.CancelledError:
            pass
        except Exception:
            logger.exception("sandbox.shutdown.prewarm_cleanup_failed")
        _prewarm_task = None

    if _scheduler is not None:
        try:
            await _scheduler.__aexit__(None, None, None)
            logger.info("sandbox.shutdown.ok")
        except Exception:
            logger.exception("sandbox.shutdown.failed")
        finally:
            _scheduler = None
            _initialized = False


def get_warm_pool_state() -> dict[str, dict[str, int]]:
    """
    Return current warm pool state for the readiness probe.

    Best-effort — exec-sandbox doesn't expose per-language pool counts cleanly,
    so we approximate from the total pool size if available. Operators rely
    on the /metrics endpoint (warm_pool_size gauge) for precise data;
    /health/ready is a coarse readiness signal.
    """
    if _scheduler is None:
        return {
            "python": {"target": 0, "current": 0},
            "javascript": {"target": 0, "current": 0},
        }

    # exec-sandbox internal API may differ across versions; we expose what we can.
    # If exec-sandbox adds per-language introspection, plumb it through here.
    return {
        "python": {
            "target": int(warm_pool_target.labels(language="python")._value.get()),
            "current": int(warm_pool_size.labels(language="python")._value.get()),
        },
        "javascript": {
            "target": int(warm_pool_target.labels(language="javascript")._value.get()),
            "current": int(warm_pool_size.labels(language="javascript")._value.get()),
        },
    }
