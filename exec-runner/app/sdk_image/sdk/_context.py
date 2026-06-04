"""
Lazy execution-context helpers.

These helpers are intentionally opt-in: scripts that only need sdk.input never
pay for platform reads. The values are memoized for the duration of one
execution and read through the normal sdk.api client, so auth, idempotency, and
platform cache behavior stay centralized.
"""
from __future__ import annotations

import os
from typing import Any

from . import _api, _errors

_CACHE: dict[str, dict[str, Any] | None] = {}


def _execution_id() -> str:
    return os.environ.get("_EXECUTION_ID", "")


def _conversation_uuid() -> str:
    return os.environ.get("_CONVERSATION_UUID", "")


def _cache_key(name: str) -> str:
    return f"{_execution_id()}:{name}"


def _unwrap_data(resp: Any) -> Any:
    """Unwrap the platform envelope returned by fastglue SendEnvelope."""
    if isinstance(resp, dict) and "data" in resp:
        return resp.get("data")
    return resp


def conversation() -> dict[str, Any] | None:
    """
    Return the current conversation record, or None outside conversation scope.
    """
    key = _cache_key("conversation")
    if key in _CACHE:
        return _CACHE[key]

    uuid = _conversation_uuid()
    if not uuid:
        _CACHE[key] = None
        return None

    try:
        data = _unwrap_data(_api.get(f"/api/v1/conversations/{uuid}", system=True))
    except _errors.NotFoundError:
        data = None

    _CACHE[key] = data if isinstance(data, dict) else None
    return _CACHE[key]


def contract() -> dict[str, Any] | None:
    """
    Return the linked contract record, or None when absent/unverified.
    """
    key = _cache_key("contract")
    if key in _CACHE:
        return _CACHE[key]

    conv = conversation()
    if not conv or int(conv.get("verification_level") or 0) <= 0 or not conv.get("contract_id"):
        _CACHE[key] = None
        return None

    uuid = _conversation_uuid()
    try:
        data = _unwrap_data(_api.get(f"/api/v1/conversations/{uuid}/contract", system=True))
    except _errors.NotFoundError:
        data = None

    _CACHE[key] = data if isinstance(data, dict) else None
    return _CACHE[key]


def customer() -> dict[str, Any] | None:
    """
    Return the linked customer record, or None when absent/unverified.
    """
    key = _cache_key("customer")
    if key in _CACHE:
        return _CACHE[key]

    conv = conversation()
    if not conv or int(conv.get("verification_level") or 0) <= 0 or not conv.get("customer_id"):
        _CACHE[key] = None
        return None

    uuid = _conversation_uuid()
    try:
        data = _unwrap_data(_api.get(f"/api/v1/conversations/{uuid}/customer", system=True))
    except _errors.NotFoundError:
        data = None

    _CACHE[key] = data if isinstance(data, dict) else None
    return _CACHE[key]
