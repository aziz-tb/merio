"""
Result writers — scripts call exactly one of these to report their outcome.

Implementation: emit a single line to stdout prefixed with __RESULT__ and
followed by JSON. The Python service parses the LAST such line in the
captured stdout. If a script calls multiple writers, last write wins.

If a script never calls any writer and exits 0, the service treats it as
NO_RESULT_EMITTED (a script bug).
"""
from __future__ import annotations

import json
import sys
from typing import Any

_SENTINEL = "__RESULT__"


def _emit(envelope: dict[str, Any]) -> None:
    """Print the sentinel line to stdout, force-flush so it's never buffered out."""
    line = f"{_SENTINEL} {json.dumps(envelope, default=str, ensure_ascii=False)}"
    print(line, flush=True)
    # Defensive flush in case print's flush kwarg was overridden by user code.
    try:
        sys.stdout.flush()
    except Exception:
        pass


def success(data: dict[str, Any] | None = None) -> None:
    """
    Report successful completion.

    `data` is the structured payload the platform stores as the tool's result.
    Defaults to {} if not supplied.
    """
    _emit({"kind": "success", "data": data or {}})


def failure(message: str, code: str | None = None) -> None:
    """
    Report a controlled failure — the script ran correctly but the operation
    couldn't complete (e.g., "customer not eligible", "no contract found").

    Distinct from `error`: failures are expected outcomes; errors are unexpected.
    """
    _emit({"kind": "failure", "message": message, "code": code})


def error(message: str, retryable: bool = False) -> None:
    """
    Report an unexpected error — something went wrong the script couldn't
    handle (e.g., upstream API returned 500).

    `retryable=True` signals to the caller that retrying may help.
    """
    _emit({"kind": "error", "message": message, "retryable": retryable})


def noop() -> None:
    """
    Report 'I ran, nothing to do here.'

    Used by event hooks that don't always have something to report
    (e.g., a duplicate-detection hook that found no duplicate).
    """
    _emit({"kind": "noop"})
