"""
Tests for failure-mode handling in app/runner/execute.py.

These mock the exec-sandbox Scheduler so we can exercise every exception
path without spinning up real VMs. Validates that exec-sandbox exceptions
map to the correct ExecuteErrorResponse shape.

This corresponds to F2-F8, F14, F15 in the F6 plan.
"""
from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

import pytest
from exec_sandbox import (
    InputValidationError,
    PackageNotAllowedError,
    SandboxError,
    VmCapacityError,
    VmPermanentError,
    VmTimeoutError,
    VmTransientError,
)

from app.models import ExecuteErrorResponse, ExecuteRequest, ExecuteResponse
from app.runner.execute import execute, init_gate


@pytest.fixture(autouse=True)
def _setup_gate():
    """Each test starts with a fresh admission gate."""
    init_gate(max_concurrent=4, queue_size=8)


def _make_request(**overrides) -> ExecuteRequest:
    defaults = dict(
        code="print('hello')",
        language="python",
        callback_token="test_token",
        callback_base_url="http://localhost:8080",
        execution_id="exec_test_1",
        trace_id="trace_test_1",
        timeout_seconds=10,
    )
    defaults.update(overrides)
    return ExecuteRequest(**defaults)


def _mock_scheduler_run(*, exception=None, result=None) -> AsyncMock:
    """Create a mock scheduler whose .run() raises or returns as specified."""
    scheduler = SimpleNamespace()
    if exception is not None:
        scheduler.run = AsyncMock(side_effect=exception)
    else:
        scheduler.run = AsyncMock(return_value=result)
    return scheduler


def _make_run_result(*, exit_code=0, stdout="", stderr="", duration_ms=100, warm_pool_hit=True):
    """Mimic exec-sandbox's RunResult shape."""
    return SimpleNamespace(
        exit_code=exit_code,
        stdout=stdout,
        stderr=stderr,
        timing=SimpleNamespace(total_ms=duration_ms),
        warm_pool_hit=warm_pool_hit,
    )


# ─── F4: VmTimeoutError ───────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_vm_timeout_returns_retryable_error():
    scheduler = _mock_scheduler_run(exception=VmTimeoutError("guest didn't respond"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "vm_timeout"
    assert result.retryable is True


# ─── F5: VmTransientError ─────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_vm_transient_returns_retryable_error():
    scheduler = _mock_scheduler_run(exception=VmTransientError("qemu crashed"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "vm_transient"
    assert result.retryable is True


# ─── F14: VmCapacityError ─────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_vm_capacity_returns_retryable_error():
    scheduler = _mock_scheduler_run(exception=VmCapacityError("pool full"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "vm_capacity"
    assert result.retryable is True


# ─── VmPermanentError (not retryable) ─────────────────────────────────────


@pytest.mark.asyncio
async def test_vm_permanent_returns_non_retryable_error():
    scheduler = _mock_scheduler_run(exception=VmPermanentError("protocol corruption"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "vm_permanent"
    assert result.retryable is False


# ─── InputValidationError ─────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_input_validation_returns_non_retryable_error():
    scheduler = _mock_scheduler_run(exception=InputValidationError("bad code"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "input_validation"
    assert result.retryable is False


# ─── PackageNotAllowedError ───────────────────────────────────────────────


@pytest.mark.asyncio
async def test_package_not_allowed():
    scheduler = _mock_scheduler_run(exception=PackageNotAllowedError("evil_pkg"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request(packages=["evil_pkg==1.0.0"]))
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "package_not_allowed"
    assert result.retryable is False


# ─── F6: Script timeout returns normal response with exit_code=-1 ─────────


@pytest.mark.asyncio
async def test_script_timeout_returns_normal_response_with_negative_exit():
    """exec-sandbox returns exit_code=-1 (NOT an exception) for script timeouts."""
    run_result = _make_run_result(exit_code=-1, stderr="execution timed out")
    scheduler = _mock_scheduler_run(result=run_result)
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteResponse)
    assert result.exit_code == -1
    assert "timed out" in result.stderr


# ─── F7: Script crash (non-zero exit, no result) ──────────────────────────


@pytest.mark.asyncio
async def test_script_crash_returns_response_with_no_result():
    run_result = _make_run_result(
        exit_code=1,
        stdout="some debug output\n",
        stderr="Traceback (most recent call last):\n  ValueError: bad",
    )
    scheduler = _mock_scheduler_run(result=run_result)
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteResponse)
    assert result.exit_code == 1
    assert result.result is None  # no __RESULT__ on a non-zero exit


# ─── F8: Exit 0 but no __RESULT__ → NO_RESULT_EMITTED ─────────────────────


@pytest.mark.asyncio
async def test_exit_zero_without_result_yields_no_result_emitted():
    run_result = _make_run_result(exit_code=0, stdout="just some logs\n")
    scheduler = _mock_scheduler_run(result=run_result)
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteResponse)
    assert result.exit_code == 0
    assert result.result is not None
    assert result.result.kind == "error"
    assert result.result.code == "NO_RESULT_EMITTED"


# ─── Happy path: success result is parsed correctly ───────────────────────


@pytest.mark.asyncio
async def test_success_result_parsed():
    run_result = _make_run_result(
        exit_code=0,
        stdout='__RESULT__ {"kind": "success", "data": {"x": 1}}\n',
    )
    scheduler = _mock_scheduler_run(result=run_result)
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteResponse)
    assert result.exit_code == 0
    assert result.result is not None
    assert result.result.kind == "success"
    assert result.result.data == {"x": 1}


@pytest.mark.asyncio
async def test_failure_result_parsed():
    run_result = _make_run_result(
        exit_code=0,
        stdout='__RESULT__ {"kind": "failure", "message": "no contract", "code": "NO_CONTRACT"}\n',
    )
    scheduler = _mock_scheduler_run(result=run_result)
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteResponse)
    assert result.result is not None
    assert result.result.kind == "failure"
    assert result.result.message == "no contract"
    assert result.result.code == "NO_CONTRACT"


# ─── Catch-all SandboxError ───────────────────────────────────────────────


@pytest.mark.asyncio
async def test_generic_sandbox_error():
    scheduler = _mock_scheduler_run(exception=SandboxError("unknown failure"))
    with patch("app.runner.execute.get_scheduler", return_value=scheduler):
        result = await execute(_make_request())
    assert isinstance(result, ExecuteErrorResponse)
    assert result.error == "sandbox_error"
    assert result.retryable is False
