"""
Structured JSON logging via structlog.

Service logs go to stdout. Trace IDs propagate via context binding so that
all logs for one request share the same trace_id.

Script stdout/stderr are NOT logged here — they are returned in the response
envelope. Reason: scripts may legitimately log PII or business data.
"""
from __future__ import annotations

import logging
import sys

import structlog


def setup_logging(*, level: str = "INFO", log_format: str = "json") -> None:
    """
    Configure structlog and the stdlib logger.

    Call once at startup. Idempotent — safe to call multiple times.
    """
    # Stdlib logger config — captures third-party library logs (uvicorn, exec-sandbox, etc.)
    logging.basicConfig(
        format="%(message)s",
        stream=sys.stdout,
        level=getattr(logging, level.upper(), logging.INFO),
        force=True,
    )

    # Quiet noisy third-party loggers
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.getLogger("exec_sandbox.warm_vm_pool").setLevel(logging.WARNING)

    shared_processors = [
        structlog.contextvars.merge_contextvars,
        structlog.processors.add_log_level,
        structlog.processors.TimeStamper(fmt="iso", utc=True),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
    ]

    if log_format == "json":
        renderer = structlog.processors.JSONRenderer()
    else:
        renderer = structlog.dev.ConsoleRenderer(colors=sys.stdout.isatty())

    structlog.configure(
        processors=shared_processors + [renderer],
        wrapper_class=structlog.make_filtering_bound_logger(
            getattr(logging, level.upper(), logging.INFO)
        ),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )


def get_logger(name: str | None = None) -> structlog.stdlib.BoundLogger:
    """Return a bound logger. Pass __name__ for module-level loggers."""
    return structlog.get_logger(name) if name else structlog.get_logger()


def bind_trace(trace_id: str) -> None:
    """
    Bind trace_id to the current async context.

    All subsequent log calls in this context will include trace_id.
    Cleared automatically when the context ends.
    """
    structlog.contextvars.bind_contextvars(trace_id=trace_id)


def clear_trace() -> None:
    """Clear context-bound trace data. Call at end of request."""
    structlog.contextvars.clear_contextvars()
