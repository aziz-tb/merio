"""
Data exposed to scripts via env vars, populated by the Python service before
each execution.

Scripts read these as plain dicts (or None for optional context):

    sdk.input.get("contract_id")
    sdk.ticket_data.get("subject")
    if sdk.contract_data is not None:
        rate = sdk.contract_data.get("rate")

We deliberately keep these as dicts (not typed objects) so the platform's
data shapes can evolve without bumping the SDK.
"""
from __future__ import annotations

import json
import os
from typing import Any


def _load_dict(env_name: str, *, allow_null: bool) -> dict[str, Any] | None:
    """
    Decode a JSON-encoded dict from an env var.

    Returns {} if the env var is unset or empty (when allow_null=False),
    or None when allow_null=True. Raises if the env var is set to something
    that isn't a JSON object — that's a service bug, not a script bug.
    """
    raw = os.environ.get(env_name)
    if raw is None or raw == "":
        return None if allow_null else {}

    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError as e:
        raise RuntimeError(
            f"SDK init: env var {env_name} is not valid JSON: {e}"
        ) from e

    if parsed is None:
        return None if allow_null else {}

    if not isinstance(parsed, dict):
        raise RuntimeError(
            f"SDK init: env var {env_name} must decode to an object, got {type(parsed).__name__}"
        )

    return parsed


# input is always a dict (possibly empty) — never None.
input: dict[str, Any] = _load_dict("_INPUT_DATA", allow_null=False) or {}

# Context data may be absent (e.g., a script that doesn't have a contract resolved).
ticket_data: dict[str, Any] | None = _load_dict("_TICKET_DATA", allow_null=True)
contract_data: dict[str, Any] | None = _load_dict("_CONTRACT_DATA", allow_null=True)
customer_data: dict[str, Any] | None = _load_dict("_CUSTOMER_DATA", allow_null=True)
