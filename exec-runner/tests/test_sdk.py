"""
Tests for the SDK module that runs INSIDE the VM.

We exercise the SDK directly by setting the env vars it expects and
mocking the requests library. This validates idempotency-key derivation,
error mapping, and data accessors without spinning up a real VM.
"""
from __future__ import annotations

import importlib
import json
import os
import sys
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# Add the SDK image dir to sys.path so we can `import sdk` directly.
SDK_IMAGE = Path(__file__).parent.parent / "app" / "sdk_image"
sys.path.insert(0, str(SDK_IMAGE))


def _reload_sdk():
    """Re-import the sdk module after env changes — needed because data accessors
    are evaluated at import time."""
    # Clear cached sdk modules so they re-read env on next import.
    for mod_name in list(sys.modules):
        if mod_name == "sdk" or mod_name.startswith("sdk."):
            del sys.modules[mod_name]
    import sdk  # noqa: F401
    return sys.modules["sdk"]


# ─── Data accessors ───────────────────────────────────────────────────────


def test_input_loaded_from_env(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", json.dumps({"contract_id": 64168552}))
    monkeypatch.setenv("_TICKET_DATA", "null")
    monkeypatch.setenv("_CONTRACT_DATA", "null")
    monkeypatch.setenv("_CUSTOMER_DATA", "null")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.input == {"contract_id": 64168552}
    assert sdk.input.get("contract_id") == 64168552
    assert sdk.ticket_data is None
    assert sdk.contract_data is None
    assert sdk.customer_data is None


def test_ticket_data_loaded_when_present(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_TICKET_DATA", json.dumps({"id": 42, "subject": "Hello"}))
    monkeypatch.setenv("_CONTRACT_DATA", "null")
    monkeypatch.setenv("_CUSTOMER_DATA", "null")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.ticket_data == {"id": 42, "subject": "Hello"}


def test_input_empty_when_env_unset(monkeypatch):
    monkeypatch.delenv("_INPUT_DATA", raising=False)
    monkeypatch.delenv("_TICKET_DATA", raising=False)
    monkeypatch.delenv("_CONTRACT_DATA", raising=False)
    monkeypatch.delenv("_CUSTOMER_DATA", raising=False)
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.input == {}
    assert sdk.ticket_data is None


# ─── Output writing ───────────────────────────────────────────────────────


def test_success_emits_sentinel(monkeypatch, capsys):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    sdk.success({"refund_id": "r_123"})

    captured = capsys.readouterr()
    assert "__RESULT__" in captured.out
    line = [l for l in captured.out.splitlines() if l.startswith("__RESULT__")][-1]
    payload = json.loads(line.removeprefix("__RESULT__").strip())
    assert payload["kind"] == "success"
    assert payload["data"] == {"refund_id": "r_123"}


def test_failure_with_code(monkeypatch, capsys):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    sdk.failure("not eligible", code="INELIGIBLE")

    captured = capsys.readouterr()
    line = [l for l in captured.out.splitlines() if l.startswith("__RESULT__")][-1]
    payload = json.loads(line.removeprefix("__RESULT__").strip())
    assert payload["kind"] == "failure"
    assert payload["message"] == "not eligible"
    assert payload["code"] == "INELIGIBLE"


def test_error_with_retryable(monkeypatch, capsys):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    sdk.error("upstream timeout", retryable=True)

    captured = capsys.readouterr()
    line = [l for l in captured.out.splitlines() if l.startswith("__RESULT__")][-1]
    payload = json.loads(line.removeprefix("__RESULT__").strip())
    assert payload["kind"] == "error"
    assert payload["retryable"] is True


def test_noop(monkeypatch, capsys):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    sdk.noop()

    captured = capsys.readouterr()
    line = [l for l in captured.out.splitlines() if l.startswith("__RESULT__")][-1]
    payload = json.loads(line.removeprefix("__RESULT__").strip())
    assert payload["kind"] == "noop"


# ─── Idempotency key derivation ───────────────────────────────────────────


def test_idempotency_key_stable_for_same_inputs(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()

    # Inspect the internal helper directly to verify key stability.
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})
    assert h1["Idempotency-Key"] == h2["Idempotency-Key"]
    assert len(h1["Idempotency-Key"]) == 32  # SHA256 hex truncated to 32


def test_idempotency_key_differs_for_different_bodies(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "urgent"})
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_idempotency_key_differs_for_different_paths(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})
    h2 = sdk.api._write_headers("/sdk/tickets/2/tags", {"tag": "vip"})
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_idempotency_key_differs_for_different_executions(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")

    monkeypatch.setenv("_EXECUTION_ID", "exec_111")
    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})

    monkeypatch.setenv("_EXECUTION_ID", "exec_222")
    sdk = _reload_sdk()
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"})
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_get_has_no_idempotency_header(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h = sdk.api._read_headers()
    assert "Idempotency-Key" not in h


# ─── Error mapping ────────────────────────────────────────────────────────


def _mock_response(status_code: int, body: str = "", json_body=None):
    resp = MagicMock()
    resp.status_code = status_code
    # If a JSON body is provided but no raw body string, synthesize the bytes
    # so that resp.content is truthy — _handle() returns None for empty content.
    if json_body is not None and not body:
        body = json.dumps(json_body)
    resp.content = body.encode() if body else b""
    resp.text = body
    resp.headers = {"content-type": "application/json" if json_body else "text/plain"}
    if json_body is not None:
        resp.json.return_value = json_body
    return resp


def test_get_404_raises_not_found(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.requests.get", return_value=_mock_response(404, "not found")):
        with pytest.raises(sdk.NotFoundError):
            sdk.api.get("/sdk/tickets/999")


def test_post_403_raises_permission(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.requests.post", return_value=_mock_response(403, "forbidden")):
        with pytest.raises(sdk.PermissionError):
            sdk.api.post("/sdk/refunds", {"amount": 50})


def test_post_500_raises_platform_error(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.requests.post", return_value=_mock_response(500, "boom")):
        with pytest.raises(sdk.PlatformError):
            sdk.api.post("/sdk/refunds", {"amount": 50})


def test_get_returns_parsed_json(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    expected = {"id": 123, "subject": "Hello"}
    with patch(
        "sdk._api.requests.get",
        return_value=_mock_response(200, body="ignored", json_body=expected),
    ):
        result = sdk.api.get("/sdk/tickets/123")
        assert result == expected


def test_post_with_body_sends_idempotency_header(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    captured_headers = {}

    def _capture(*args, **kwargs):
        captured_headers.update(kwargs.get("headers") or {})
        return _mock_response(200, json_body={"ok": True})

    with patch("sdk._api.requests.post", side_effect=_capture):
        sdk.api.post("/sdk/tickets/1/tags", {"tag": "vip"})

    assert "Idempotency-Key" in captured_headers
    assert "Authorization" in captured_headers
    assert captured_headers["Authorization"] == "Bearer test"


# ─── Tools depth control ──────────────────────────────────────────────────


def test_tools_call_at_max_depth_raises(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")
    monkeypatch.setenv("_TOOL_DEPTH", "3")  # at the cap

    sdk = _reload_sdk()
    with pytest.raises(sdk.DepthLimitError):
        sdk.tools.call("anything", {})


def test_tools_call_under_depth_dispatches(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")
    monkeypatch.setenv("_TOOL_DEPTH", "1")

    sdk = _reload_sdk()
    with patch(
        "sdk._api.requests.post",
        return_value=_mock_response(200, json_body={"data": "ok"}),
    ):
        result = sdk.tools.call("search", {"q": "x"})
        assert result == {"data": "ok"}
