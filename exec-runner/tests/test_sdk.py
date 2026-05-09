"""
Tests for the SDK module that runs INSIDE the VM.

We exercise the SDK directly by setting the env vars it expects and
mocking urllib.request.urlopen. This validates idempotency-key derivation,
error mapping, and data accessors without spinning up a real VM.
"""
from __future__ import annotations

import importlib
import io
import json
import os
import sys
import urllib.error
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
    monkeypatch.setenv("_CONVERSATION_DATA", "null")
    monkeypatch.setenv("_CONTRACT_DATA", "null")
    monkeypatch.setenv("_CUSTOMER_DATA", "null")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.input == {"contract_id": 64168552}
    assert sdk.input.get("contract_id") == 64168552
    assert sdk.conversation_data is None
    assert sdk.contract_data is None
    assert sdk.customer_data is None


def test_conversation_data_loaded_when_present(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CONVERSATION_DATA", json.dumps({"id": 42, "subject": "Hello"}))
    monkeypatch.setenv("_CONTRACT_DATA", "null")
    monkeypatch.setenv("_CUSTOMER_DATA", "null")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.conversation_data == {"id": 42, "subject": "Hello"}


def test_input_empty_when_env_unset(monkeypatch):
    monkeypatch.delenv("_INPUT_DATA", raising=False)
    monkeypatch.delenv("_CONVERSATION_DATA", raising=False)
    monkeypatch.delenv("_CONTRACT_DATA", raising=False)
    monkeypatch.delenv("_CUSTOMER_DATA", raising=False)
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_123")

    sdk = _reload_sdk()
    assert sdk.input == {}
    assert sdk.conversation_data is None


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
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)
    assert h1["Idempotency-Key"] == h2["Idempotency-Key"]
    assert len(h1["Idempotency-Key"]) == 32  # SHA256 hex truncated to 32


def test_idempotency_key_differs_for_different_bodies(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "urgent"}, False)
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_idempotency_key_differs_for_different_paths(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)
    h2 = sdk.api._write_headers("/sdk/tickets/2/tags", {"tag": "vip"}, False)
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_idempotency_key_differs_for_different_executions(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")

    monkeypatch.setenv("_EXECUTION_ID", "exec_111")
    sdk = _reload_sdk()
    h1 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)

    monkeypatch.setenv("_EXECUTION_ID", "exec_222")
    sdk = _reload_sdk()
    h2 = sdk.api._write_headers("/sdk/tickets/1/tags", {"tag": "vip"}, False)
    assert h1["Idempotency-Key"] != h2["Idempotency-Key"]


def test_get_has_no_idempotency_header(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    h = sdk.api._read_headers(False)
    assert "Idempotency-Key" not in h


# ─── Error mapping ────────────────────────────────────────────────────────


class _FakeResponse:
    """
    Stand-in for the object returned by urllib.request.urlopen() on 2xx.
    Implements the context manager protocol and the attributes our SDK reads.
    """

    def __init__(self, status: int, body: bytes = b"", content_type: str = "application/json"):
        self.status = status
        self._body = body
        self.headers = {"content-type": content_type}

    def read(self) -> bytes:
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        return False


def _make_http_error(status: int, body: str = "") -> urllib.error.HTTPError:
    """Build an HTTPError that urllib raises for 4xx/5xx responses."""
    return urllib.error.HTTPError(
        url="http://localhost:8080/test",
        code=status,
        msg=body or "error",
        hdrs={},
        fp=io.BytesIO(body.encode("utf-8")) if body else io.BytesIO(b""),
    )


def test_get_404_raises_not_found(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.urllib.request.urlopen", side_effect=_make_http_error(404, "not found")):
        with pytest.raises(sdk.NotFoundError):
            sdk.api.get("/sdk/tickets/999")


def test_post_403_raises_permission(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.urllib.request.urlopen", side_effect=_make_http_error(403, "forbidden")):
        with pytest.raises(sdk.PermissionError):
            sdk.api.post("/sdk/refunds", {"amount": 50})


def test_post_500_raises_platform_error(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    with patch("sdk._api.urllib.request.urlopen", side_effect=_make_http_error(500, "boom")):
        with pytest.raises(sdk.PlatformError):
            sdk.api.post("/sdk/refunds", {"amount": 50})


def test_get_returns_parsed_json(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    expected = {"id": 123, "subject": "Hello"}
    fake_resp = _FakeResponse(200, json.dumps(expected).encode("utf-8"))
    with patch("sdk._api.urllib.request.urlopen", return_value=fake_resp):
        result = sdk.api.get("/sdk/tickets/123")
        assert result == expected


def test_post_with_body_sends_idempotency_header(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CALLBACK_BASE_URL", "http://localhost:8080")
    monkeypatch.setenv("_CALLBACK_TOKEN", "test")
    monkeypatch.setenv("_EXECUTION_ID", "exec_abc")

    sdk = _reload_sdk()
    captured_request = {}

    def _capture(req, *args, **kwargs):
        # urllib normalizes header keys (Idempotency-Key → Idempotency-key).
        # We read via .get_header() and .header_items() which are case-insensitive.
        captured_request["headers"] = dict(req.header_items())
        captured_request["method"] = req.get_method()
        captured_request["url"] = req.full_url
        return _FakeResponse(200, b'{"ok": true}')

    with patch("sdk._api.urllib.request.urlopen", side_effect=_capture):
        sdk.api.post("/sdk/tickets/1/tags", {"tag": "vip"})

    headers = captured_request["headers"]
    # urllib title-cases header names: "Idempotency-Key" → "Idempotency-key"
    header_keys_lower = {k.lower() for k in headers}
    assert "idempotency-key" in header_keys_lower
    assert "authorization" in header_keys_lower
    auth_value = next(v for k, v in headers.items() if k.lower() == "authorization")
    # New SDK uses Token <key>:<secret> shape, not Bearer.
    assert auth_value.startswith("Token ")
    assert captured_request["method"] == "POST"


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
    # tools.call unwraps two layers: the outer fastglue {"data": ...}
    # envelope, then the runner's {"kind": "success", "data": <payload>}
    # envelope. So the wire body needs both wrappers to surface <payload>.
    fake_resp = _FakeResponse(
        200, b'{"data": {"kind": "success", "data": {"hits": ["a", "b"]}}}'
    )
    with patch("sdk._api.urllib.request.urlopen", return_value=fake_resp):
        result = sdk.tools.call("search", {"q": "x"})
        assert result == {"hits": ["a", "b"]}
