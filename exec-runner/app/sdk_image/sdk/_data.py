"""
Data exposed to scripts via env vars, populated by the Python service before
each execution.

Scripts read input as a plain dict:

    sdk.input.get("contract_id")

Conversation, contract, and customer context is available through the lazy
sdk.context helpers, not top-level eager data fields.

Naming notes
------------
- secrets: operator-managed plaintext secrets passed by the platform for
  this execution. The public SDK exposes read-only access through
  sdk.secret.get(name), not as a top-level data object.
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

secrets: dict[str, Any] = _load_dict("_SECRETS_DATA", allow_null=False) or {}
