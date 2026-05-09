"""
Tool composition — call other tools through the platform's tool registry.

A script can compose:
    result = sdk.tools.call("search_glossary", {"term": "BNetzA"})

This dispatches to the Go-side tool registry, which invokes the named tool
with proper audit, policy, and idempotency. The called tool can be a Go
tool or another Python script.

Depth control: each call increments _TOOL_DEPTH. Capped at _MAX_DEPTH to
prevent runaway loops (script A calls B calls A calls B ...). The cap is
shared between the SDK (raises DepthLimitError before the call) and the
Go side (refuses to dispatch beyond the cap).

Return value: on `kind == "success"` the called tool's data dict is
returned directly. On any other kind (failure, error, noop) the SDK
raises an exception so the calling script can use try/except naturally:

    try:
        invoices = sdk.tools.call("fetch_invoices", {"customer_id": 99})
    except sdk.ApiError as e:
        # Called tool returned failure/error/noop
        sdk.failure(f"could not fetch invoices: {e}")
"""
from __future__ import annotations

import os
from typing import Any

from . import _api, _errors

_MAX_DEPTH = 3


def call(name: str, args: dict[str, Any], *, system: bool = False) -> Any:
    """
    Invoke another tool by name through the Go tool registry.

    Args:
        name: Registered tool name.
        args: JSON-serializable dict passed to the called tool as
            sdk.input.
        system: When True, the dispatched call runs as the System user
            instead of inheriting the caller's invoker. Note this
            applies to the dispatch *call itself* — the called tool
            receives its own auth context and decides per-call whether
            to use system=True too.

    Returns:
        The called tool's success.data dict.

    Raises:
        DepthLimitError: when the call would exceed the recursion cap.
        NotFoundError: when no tool is registered with this name.
        PermissionError: when the call is not authorized.
        ApiError: when the called tool returned a non-success kind
            (failure, error, or noop). The exception's message includes
            the kind, code, and message from the result envelope.
    """
    depth = int(os.environ.get("_TOOL_DEPTH", "0"))
    if depth >= _MAX_DEPTH:
        raise _errors.DepthLimitError(
            f"sdk.tools.call depth {depth} exceeds maximum {_MAX_DEPTH}"
        )

    envelope = _api.post(
        f"/api/v1/tools/{name}/call",
        {"args": args},
        system=system,
    )

    # The platform wraps responses in {"data": ...} (envelope shape from
    # fastglue's SendEnvelope). Unwrap one layer to get the actual result
    # envelope produced by the runner.
    result = envelope.get("data") if isinstance(envelope, dict) else envelope
    if not isinstance(result, dict):
        raise _errors.PlatformError(
            f"sdk.tools.call({name}): unexpected response shape {type(result).__name__}"
        )

    kind = result.get("kind")
    if kind == "success":
        return result.get("data") or {}

    # Anything else is an error to the caller. Build a useful message.
    message = result.get("message") or ""
    code = result.get("code") or ""
    detail = f"kind={kind}"
    if code:
        detail += f" code={code}"
    if message:
        detail += f": {message}"

    raise _errors.ApiError(f"sdk.tools.call({name}): {detail}")
