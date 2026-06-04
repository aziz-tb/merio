"""
The execute() function — orchestrates a single script run.

Flow:
  1. Build env vars from request payload (data + callback config)
  2. Wrap user code with SDK preamble (writes SDK files into VM, sets sys.path)
  3. Call exec-sandbox scheduler.run(...)
  4. Parse the __RESULT__ sentinel from stdout
  5. Map exec-sandbox exceptions to our error envelope

This is the only place where exec-sandbox APIs are called. Everywhere else
in the service treats this layer as opaque.
"""
from __future__ import annotations

import asyncio
import json
from pathlib import Path
from typing import Any

from exec_sandbox import (
    InputValidationError,
    PackageNotAllowedError,
    SandboxError,
    VmCapacityError,
    VmPermanentError,
    VmTimeoutError,
    VmTransientError,
)

from app.models import ExecuteErrorResponse, ExecuteRequest, ExecuteResponse, ExecuteResultEnvelope
from app.observability.logging import get_logger
from app.observability.metrics import (
    execution_duration_ms,
    executions_total,
    vm_errors_total,
)
from app.runner.parsing import parse_result_line
from app.runner.scheduler import get_scheduler

logger = get_logger(__name__)

# ─── SDK embedding ────────────────────────────────────────────────────────
#
# exec-sandbox's scheduler.run() does NOT support file uploads (only
# scheduler.session() does, via write_file()). To make `import sdk` work
# inside a one-shot run, we embed the SDK source files in a Python preamble
# that writes them to /tmp/sdk_pkg/ inside the VM, then adds that to sys.path.
#
# The SDK files are read once at module load and cached. The preamble adds
# a few KB to every script — well within the 1MB code limit.

_SDK_SOURCE_DIR = Path(__file__).parent.parent / "sdk_image" / "sdk"


def _load_sdk_files() -> dict[str, str]:
    """Read every SDK *.py file once at module load."""
    files: dict[str, str] = {}
    for path in sorted(_SDK_SOURCE_DIR.glob("*.py")):
        files[path.name] = path.read_text(encoding="utf-8")
    if not files:
        # Service is broken without the SDK — fail loudly at startup
        raise RuntimeError(
            f"SDK source directory is empty: {_SDK_SOURCE_DIR}"
        )
    return files


_SDK_FILES: dict[str, str] = _load_sdk_files()


def _build_python_preamble() -> str:
    """
    Build the Python preamble that materializes the SDK package inside the VM.

    Uses repr() for each file content to safely escape arbitrary text (quotes,
    newlines, unicode) into a Python string literal.
    """
    files_dict_literal = ",\n    ".join(
        f"{name!r}: {content!r}" for name, content in _SDK_FILES.items()
    )
    return f"""\
import os as _sdk_os, sys as _sdk_sys
_sdk_os.makedirs("/tmp/sdk_pkg/sdk", exist_ok=True)
_SDK_FILES = {{
    {files_dict_literal}
}}
for _sdk_name, _sdk_content in _SDK_FILES.items():
    with open(f"/tmp/sdk_pkg/sdk/{{_sdk_name}}", "w", encoding="utf-8") as _sdk_f:
        _sdk_f.write(_sdk_content)
del _SDK_FILES, _sdk_name, _sdk_content, _sdk_f, _sdk_os
if "/tmp/sdk_pkg" not in _sdk_sys.path:
    _sdk_sys.path.insert(0, "/tmp/sdk_pkg")
del _sdk_sys
"""


_PYTHON_PREAMBLE: str = _build_python_preamble()
_JS_PREAMBLE = ""  # JavaScript SDK is v2; for now JS scripts run without sdk


def _wrap_code(code: str, language: str) -> str:
    if language == "python":
        return _PYTHON_PREAMBLE + "\n" + code
    elif language == "javascript":
        return _JS_PREAMBLE + code
    raise ValueError(f"Unsupported language: {language}")


# ─── Env builder ───────────────────────────────────────────────────────────


def _build_env(req: ExecuteRequest) -> dict[str, str]:
    """
    Convert request payload into env vars consumed by the SDK inside the VM.

    All values are JSON-encoded strings — the SDK parses them on first access.
    """
    return {
        "_INPUT_DATA": json.dumps(req.input, default=str),
        "_CONVERSATION_UUID": req.conversation_uuid or "",
        "_SECRETS_DATA": json.dumps(req.secrets, default=str),
        "_CALLBACK_TOKEN": req.callback_token,
        "_CALLBACK_BASE_URL": req.callback_base_url,
        "_EXECUTION_ID": req.execution_id,
        "_TOOL_DEPTH": str(req.tool_depth),
        "_TRACE_ID": req.trace_id,
    }


# ─── Concurrency control ──────────────────────────────────────────────────


class ExecutionGate:
    """
    Service-level concurrency cap with a bounded admission queue.

    Sits above exec-sandbox's own admission control as a fast-fail boundary.
    When the queue is full, returns a `vm_capacity` error immediately rather
    than letting requests pile up unbounded.
    """

    def __init__(self, max_concurrent: int, queue_size: int):
        self._max_concurrent = max_concurrent
        self._queue_size = queue_size
        # We track in-flight count separately from the semaphore for metrics.
        self._sem = asyncio.Semaphore(max_concurrent)
        self._in_flight = 0
        self._waiting = 0
        self._lock = asyncio.Lock()

    async def __aenter__(self) -> None:
        async with self._lock:
            if self._waiting >= self._queue_size:
                raise _AdmissionFull()
            self._waiting += 1

        try:
            await self._sem.acquire()
        finally:
            async with self._lock:
                self._waiting -= 1

        async with self._lock:
            self._in_flight += 1

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:  # type: ignore[no-untyped-def]
        async with self._lock:
            self._in_flight -= 1
        self._sem.release()

    @property
    def in_flight(self) -> int:
        return self._in_flight

    @property
    def waiting(self) -> int:
        return self._waiting

    @property
    def max_concurrent(self) -> int:
        return self._max_concurrent


class _AdmissionFull(Exception):
    """Raised when the admission queue is full."""


# Module-level gate — initialized in main.py lifespan
_gate: ExecutionGate | None = None


def init_gate(max_concurrent: int, queue_size: int) -> None:
    global _gate
    _gate = ExecutionGate(max_concurrent, queue_size)


def get_gate() -> ExecutionGate:
    if _gate is None:
        raise RuntimeError("ExecutionGate not initialized")
    return _gate


# ─── Main execute function ────────────────────────────────────────────────


async def execute(req: ExecuteRequest) -> ExecuteResponse | ExecuteErrorResponse:
    """
    Run a single script. Returns either a normal response (script ran, regardless
    of script success/failure) or an error response (sandbox-level failure).
    """
    scheduler = get_scheduler()
    env = _build_env(req)
    wrapped_code = _wrap_code(req.code, req.language)

    # Acquire admission slot (or fail fast if queue full)
    gate = get_gate()
    try:
        async with gate:
            return await _execute_inside_gate(
                scheduler=scheduler,
                req=req,
                env=env,
                wrapped_code=wrapped_code,
            )
    except _AdmissionFull:
        vm_errors_total.labels(error_type="admission_full").inc()
        logger.warning(
            "execute.admission_full",
            in_flight=gate.in_flight,
            waiting=gate.waiting,
            max_concurrent=gate.max_concurrent,
        )
        return ExecuteErrorResponse(
            error="vm_capacity",
            message="admission queue full",
            retryable=True,
        )


async def _execute_inside_gate(
    *,
    scheduler,  # type: ignore[no-untyped-def]
    req: ExecuteRequest,
    env: dict[str, str],
    wrapped_code: str,
) -> ExecuteResponse | ExecuteErrorResponse:
    """The hot path — runs once we've acquired an admission slot."""
    try:
        result = await scheduler.run(
            code=wrapped_code,
            language=req.language,
            packages=req.packages or [],
            timeout_seconds=req.timeout_seconds,
            memory_mb=req.memory_mb,
            env_vars=env,
            allow_network=True,
            # Single-tenant deployment: operator-authored tools get open
            # outbound network. Keep req.allowed_domains on the wire for
            # compatibility, but do not enforce it here.
            allowed_domains=[],
        )
    except VmTimeoutError as e:
        # Subclass of VmTransientError. VM never booted. Safe to retry.
        # MUST be caught before VmTransientError.
        vm_errors_total.labels(error_type="vm_timeout").inc()
        logger.warning("execute.vm_timeout", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="vm_timeout", message=str(e), retryable=True)
    except VmCapacityError as e:
        # Subclass of VmTransientError. Pool at capacity. Retry with backoff.
        # MUST be caught before VmTransientError.
        vm_errors_total.labels(error_type="vm_capacity").inc()
        logger.warning("execute.vm_capacity", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="vm_capacity", message=str(e), retryable=True)
    except VmTransientError as e:
        # QEMU crash, REPL spawn OOM, transport failure. Code never ran.
        vm_errors_total.labels(error_type="vm_transient").inc()
        logger.warning("execute.vm_transient", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="vm_transient", message=str(e), retryable=True)
    except VmPermanentError as e:
        # Protocol corruption. NOT retryable.
        vm_errors_total.labels(error_type="vm_permanent").inc()
        logger.error("execute.vm_permanent", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="vm_permanent", message=str(e), retryable=False)
    except InputValidationError as e:
        # Caller bug — bad code or env vars. NOT retryable.
        vm_errors_total.labels(error_type="input_validation").inc()
        logger.warning("execute.input_validation", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="input_validation", message=str(e), retryable=False)
    except PackageNotAllowedError as e:
        vm_errors_total.labels(error_type="package_not_allowed").inc()
        logger.warning("execute.package_not_allowed", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="package_not_allowed", message=str(e), retryable=False)
    except SandboxError as e:
        # Catch-all. Treat as non-retryable since we don't know the cause.
        vm_errors_total.labels(error_type="sandbox_error").inc()
        logger.error("execute.sandbox_error", trace_id=req.trace_id, error=str(e))
        return ExecuteErrorResponse(error="sandbox_error", message=str(e), retryable=False)

    # Normal path — script ran (successfully or not)
    parsed = parse_result_line(result.stdout)
    duration_ms = int(result.timing.total_ms) if hasattr(result, "timing") else 0
    warm_pool_hit = bool(getattr(result, "warm_pool_hit", False))

    # Convert parsed dict to typed envelope
    result_envelope: ExecuteResultEnvelope | None = None
    if parsed is not None:
        result_envelope = ExecuteResultEnvelope(
            kind=parsed["kind"],
            data=parsed.get("data"),
            message=parsed.get("message"),
            code=parsed.get("code"),
            retryable=bool(parsed.get("retryable", False)),
        )
    elif result.exit_code == 0:
        # Script exited 0 but emitted no result — bug in the script.
        result_envelope = ExecuteResultEnvelope(
            kind="error",
            data={},
            message="script exited successfully but emitted no __RESULT__",
            code="NO_RESULT_EMITTED",
            retryable=False,
        )

    # Metrics
    result_kind = result_envelope.kind if result_envelope else "no_result"
    executions_total.labels(
        language=req.language,
        exit_code=str(result.exit_code),
        result_kind=result_kind,
    ).inc()
    execution_duration_ms.labels(language=req.language).observe(duration_ms)

    logger.info(
        "execute.done",
        trace_id=req.trace_id,
        execution_id=req.execution_id,
        language=req.language,
        exit_code=result.exit_code,
        result_kind=result_kind,
        duration_ms=duration_ms,
        warm_pool_hit=warm_pool_hit,
    )

    return ExecuteResponse(
        exit_code=result.exit_code,
        stdout=result.stdout or "",
        stderr=result.stderr or "",
        result=result_envelope,
        duration_ms=duration_ms,
        warm_pool_hit=warm_pool_hit,
    )
