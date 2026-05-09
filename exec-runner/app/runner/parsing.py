"""
Parse script stdout for the __RESULT__ sentinel line.

Scripts call sdk.success/failure/error/noop, which emits a single line
prefixed with __RESULT__ followed by JSON. We scan stdout for the LAST such
line and parse it. Anything else on stdout is ignored at this layer (it's
returned in the response envelope as raw stdout for the Go side to log).

If multiple sentinels appear, we keep the last — the script reassigned its
result, last write wins.
"""
from __future__ import annotations

import json
from typing import Any

from app.observability.logging import get_logger

logger = get_logger(__name__)

SENTINEL = "__RESULT__"


def parse_result_line(stdout: str) -> dict[str, Any] | None:
    """
    Return the parsed result envelope, or None if no sentinel found.

    Tolerates malformed JSON on the sentinel line by returning None — the
    caller treats absence of result as NO_RESULT_EMITTED.
    """
    if not stdout:
        return None

    # Walk lines in reverse to find the LAST sentinel quickly.
    last_payload: str | None = None
    for line in reversed(stdout.splitlines()):
        line = line.rstrip()
        if line.startswith(SENTINEL):
            # Format: __RESULT__ {json}
            payload = line[len(SENTINEL):].strip()
            if payload:
                last_payload = payload
                break

    if last_payload is None:
        return None

    try:
        parsed = json.loads(last_payload)
    except json.JSONDecodeError:
        logger.warning("result.parse.invalid_json", payload_prefix=last_payload[:200])
        return None

    # Validate the envelope shape minimally.
    if not isinstance(parsed, dict):
        logger.warning("result.parse.not_object", type=type(parsed).__name__)
        return None

    kind = parsed.get("kind")
    if kind not in ("success", "failure", "error", "noop"):
        logger.warning("result.parse.invalid_kind", kind=kind)
        return None

    return parsed
