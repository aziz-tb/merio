"""
Pre-warming L1 memory snapshots so first real call hits warm cache.

Two flavors:
  - prewarm_common_snapshots() — runs in background at startup for predefined sets
  - prewarm_tool_snapshot(language, packages) — called by /v1/prewarm when an
    admin creates or updates a tool with a new package set
"""
from __future__ import annotations

from app.observability.logging import get_logger
from app.observability.metrics import prewarm_total
from app.runner.scheduler import get_scheduler

logger = get_logger(__name__)

# Common package sets to pre-warm at startup.
# Edit based on what tools commonly need. Keep tight — each adds ~30-60s to
# initial bootstrap.
COMMON_SNAPSHOTS: list[dict] = [
    {"language": "python", "packages": []},  # bare Python
    {"language": "python", "packages": ["requests==2.32.3"]},  # most common HTTP client
    {"language": "javascript", "packages": []},  # bare Bun
]


async def prewarm_common_snapshots() -> None:
    """Pre-build L1 snapshots for commonly-used package sets."""
    logger.info("prewarm.common.start", count=len(COMMON_SNAPSHOTS))

    for spec in COMMON_SNAPSHOTS:
        lang = spec["language"]
        packages = spec.get("packages", [])
        try:
            await prewarm_tool_snapshot(lang, packages, source="common")
        except Exception:
            # Don't let one failed prewarm block the rest.
            logger.exception("prewarm.common.entry_failed", language=lang, packages=packages)

    logger.info("prewarm.common.done")


async def prewarm_tool_snapshot(
    language: str,
    packages: list[str],
    *,
    source: str = "tool",
) -> bool:
    """
    Build (or refresh) the L1 snapshot for a specific package set.

    Returns True on success. Logs and returns False on failure rather than
    raising — callers (admin endpoints) should treat failure as non-fatal
    and surface to the operator.
    """
    scheduler = get_scheduler()

    try:
        result = await scheduler.run(
            code="print('snapshot_ready')",
            language=language,
            packages=packages,
            timeout_seconds=180,  # package install can be slow
        )
        ok = result.exit_code == 0

        prewarm_total.labels(
            language=language,
            outcome="success" if ok else "failed",
        ).inc()

        if ok:
            duration_ms = result.timing.total_ms if hasattr(result, "timing") else None
            logger.info(
                "prewarm.snapshot.ok",
                language=language,
                packages=packages,
                duration_ms=duration_ms,
                source=source,
            )
        else:
            logger.warning(
                "prewarm.snapshot.nonzero_exit",
                language=language,
                packages=packages,
                exit_code=result.exit_code,
                stderr=result.stderr[:200] if result.stderr else "",
                source=source,
            )
        return ok

    except Exception:
        prewarm_total.labels(language=language, outcome="error").inc()
        logger.exception(
            "prewarm.snapshot.failed",
            language=language,
            packages=packages,
            source=source,
        )
        return False
