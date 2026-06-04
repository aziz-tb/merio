import importlib
import sys
from pathlib import Path

import pytest

SDK_ROOT = Path(__file__).resolve().parents[1] / "app" / "sdk_image"


def fresh_sdk(monkeypatch):
    for name in list(sys.modules):
        if name == "sdk" or name.startswith("sdk."):
            del sys.modules[name]
    monkeypatch.syspath_prepend(str(SDK_ROOT))
    return importlib.import_module("sdk")


def test_context_conversation_returns_none_without_uuid(monkeypatch):
    monkeypatch.delenv("_CONVERSATION_UUID", raising=False)
    monkeypatch.delenv("_CONVERSATION_DATA", raising=False)
    monkeypatch.delenv("_CONTRACT_DATA", raising=False)
    monkeypatch.delenv("_CUSTOMER_DATA", raising=False)
    monkeypatch.setenv("_EXECUTION_ID", "exec-1")
    sdk = fresh_sdk(monkeypatch)

    calls = []
    monkeypatch.setattr(sdk.context._api, "get", lambda *a, **k: calls.append(a))

    assert sdk.context.conversation() is None
    assert calls == []


def test_sdk_does_not_expose_eager_context_fields(monkeypatch):
    monkeypatch.setenv("_INPUT_DATA", "{}")
    monkeypatch.setenv("_CONVERSATION_DATA", '{"uuid":"old"}')
    monkeypatch.setenv("_CONTRACT_DATA", '{"contract_id":"old"}')
    monkeypatch.setenv("_CUSTOMER_DATA", '{"customer_id":"old"}')
    sdk = fresh_sdk(monkeypatch)

    assert not hasattr(sdk, "conversation_data")
    assert not hasattr(sdk, "contract_data")
    assert not hasattr(sdk, "customer_data")


def test_context_conversation_is_memoized(monkeypatch):
    monkeypatch.setenv("_CONVERSATION_UUID", "conv-uuid")
    monkeypatch.setenv("_EXECUTION_ID", "exec-2")
    sdk = fresh_sdk(monkeypatch)

    calls = []

    def fake_get(path, *, system=False):
        calls.append((path, system))
        return {"data": {"uuid": "conv-uuid", "verification_level": 1}}

    monkeypatch.setattr(sdk.context._api, "get", fake_get)

    assert sdk.context.conversation()["uuid"] == "conv-uuid"
    assert sdk.context.conversation()["uuid"] == "conv-uuid"
    assert calls == [("/api/v1/conversations/conv-uuid", True)]


def test_context_contract_and_customer_use_linked_conversation(monkeypatch):
    monkeypatch.setenv("_CONVERSATION_UUID", "conv-uuid")
    monkeypatch.setenv("_EXECUTION_ID", "exec-3")
    sdk = fresh_sdk(monkeypatch)

    def fake_get(path, *, system=False):
        if path == "/api/v1/conversations/conv-uuid":
            return {
                "data": {
                    "uuid": "conv-uuid",
                    "verification_level": 1,
                    "contract_id": "C-1",
                    "customer_id": "K-1",
                }
            }
        if path == "/api/v1/conversations/conv-uuid/contract":
            return {"data": {"contract_id": "C-1", "rate": "green"}}
        if path == "/api/v1/conversations/conv-uuid/customer":
            return {"data": {"customer_id": "K-1", "tier": "vip"}}
        raise AssertionError(path)

    monkeypatch.setattr(sdk.context._api, "get", fake_get)

    assert sdk.context.contract() == {"contract_id": "C-1", "rate": "green"}
    assert sdk.context.customer() == {"customer_id": "K-1", "tier": "vip"}


def test_context_contract_and_customer_return_none_when_unverified(monkeypatch):
    monkeypatch.setenv("_CONVERSATION_UUID", "conv-uuid")
    monkeypatch.setenv("_EXECUTION_ID", "exec-4")
    sdk = fresh_sdk(monkeypatch)

    calls = []

    def fake_get(path, *, system=False):
        calls.append(path)
        return {
            "data": {
                "uuid": "conv-uuid",
                "verification_level": 0,
                "contract_id": "C-1",
                "customer_id": "K-1",
            }
        }

    monkeypatch.setattr(sdk.context._api, "get", fake_get)

    assert sdk.context.contract() is None
    assert sdk.context.customer() is None
    assert calls == ["/api/v1/conversations/conv-uuid"]


def test_tools_call_raises_deprecation_error(monkeypatch):
    sdk = fresh_sdk(monkeypatch)

    with pytest.raises(sdk.DeprecationError, match="sdk.tools.call has been deprecated"):
        sdk.tools.call("anything", {})
