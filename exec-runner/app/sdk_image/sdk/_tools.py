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
"""
from __future__ import annotations

import os
from typing import Any

from . import _api, _errors

_MAX_DEPTH = 3


def call(name: str, args: dict[str, Any]) -> Any:
    """
    Invoke another tool by name through the Go tool registry.

    Returns whatever the called tool's success(data=...) wrote, or raises
    ApiError if the called tool failed/errored.
    """
    depth = int(os.environ.get("_TOOL_DEPTH", "0"))
    if depth >= _MAX_DEPTH:
        raise _errors.DepthLimitError(
            f"sdk.tools.call depth {depth} exceeds maximum {_MAX_DEPTH}"
        )

    # The Go side reads the depth from the JWT or request body, increments
    # it for the new dispatch, and surfaces any errors back through the
    # standard HTTP response.
    return _api.post(f"/sdk/tools/{name}/call", {"args": args})
