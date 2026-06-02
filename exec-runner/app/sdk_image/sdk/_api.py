"""
Platform API client used inside scripts.

Scripts call:
    sdk.api.get(path, params=None, system=False)
    sdk.api.post(path, body=None, system=False)
    sdk.api.patch(path, body=None, system=False)
    sdk.api.delete(path, system=False)

By default the call acts as the script's *invoker* — the agent or
API-key user that triggered the execution. Pass `system=True` to act as
the System user instead, for actions that should not be attributed to
the human caller (auto-generated notes, system-side updates, etc.).

Idempotency on writes is automatic and internal — scripts NEVER pass an
idempotency_key argument. The SDK derives one from:
    SHA256(_EXECUTION_ID : path : body_json)[:32]
and includes it in the `Idempotency-Key` header on POST/PATCH/DELETE.

Reads have no idempotency header (naturally idempotent).

HTTP errors map to typed exceptions:
    404 → NotFoundError
    403 → PermissionError
    400 → ValidationError
    5xx → PlatformError
    other 4xx → ApiError
    timeout → TimeoutError

Implementation note: this module uses Python's stdlib `urllib` rather than
`requests` so the SDK has zero external dependencies. That matters because
exec-sandbox warm-pool VMs only contain stdlib — adding any package
(including `requests`) bypasses the warm pool and triggers a ~100ms cold
restore from L1 instead of a 1-2ms warm hit. Keeping the SDK dep-free means
every call is fast.

Scripts that need `requests` for their own external API calls can still
install it via the `packages` parameter — they just pay the cold-restore cost.

Auth shape: Merio's existing `Authorization` parser accepts
`Token <key>:<secret>`. Script tokens use the same shape — both halves
of the tuple carry the same `sx_<value>` string. The "secret" half is
meaningless for these short-lived tokens; we just match the parser's
expected format so we don't have to invent a new auth scheme.
"""
from __future__ import annotations

import hashlib
import json
import os
import socket
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

from . import _errors

# These env vars are set by the Python service before the VM runs.
_BASE = os.environ.get("_CALLBACK_BASE_URL", "").rstrip("/")
_TOKEN = os.environ.get("_CALLBACK_TOKEN", "")
_EXECUTION_ID = os.environ.get("_EXECUTION_ID", "")

# Per-call HTTP timeout. Scripts can be 60s+ but a single platform call
# should never take that long; tighter cap surfaces hangs faster.
_TIMEOUT_SECONDS = 30


# ─── Header construction ───────────────────────────────────────────────────


def _auth_value() -> str:
    """
    Builds the `Authorization` header value. Merio's parser expects
    `Token <key>:<secret>` — we send the same token in both halves; the
    server-side resolver only reads the key (the secret half is ignored
    for sx_-prefixed tokens).
    """
    return f"Token {_TOKEN}:{_TOKEN}"


def _read_headers(system: bool) -> dict[str, str]:
    """Headers for GET requests — auth + optional actor switch."""
    h = {"Authorization": _auth_value()}
    if system:
        h["X-Script-System"] = "true"
    return h


def _write_headers(path: str, body: Any | None, system: bool) -> dict[str, str]:
    """
    Headers for POST/PATCH/DELETE — adds Idempotency-Key derived from
    (execution_id, path, body) plus the optional actor switch.
    """
    body_str = (
        json.dumps(body, sort_keys=True, default=str, ensure_ascii=False)
        if body is not None
        else ""
    )
    key_input = f"{_EXECUTION_ID}:{path}:{body_str}"
    idem_key = hashlib.sha256(key_input.encode("utf-8")).hexdigest()[:32]
    h = {
        "Authorization": _auth_value(),
        "Content-Type": "application/json",
        "Idempotency-Key": idem_key,
    }
    if system:
        h["X-Script-System"] = "true"
    return h


# ─── Internal HTTP helpers ─────────────────────────────────────────────────


def _build_url(path: str, params: dict[str, Any] | None = None) -> str:
    """Build the full URL, appending query string if params provided."""
    url = _BASE + path
    if params:
        # doseq=True handles list values (repeated keys) cleanly
        url += "?" + urllib.parse.urlencode(params, doseq=True)
    return url


def _parse_success_body(resp: Any) -> Any:
    """Read and parse a 2xx response body. Returns None for empty/204."""
    status = resp.status
    if status == 204:
        return None
    raw = resp.read()
    if not raw:
        return None
    ctype = resp.headers.get("content-type", "")
    if ctype.startswith("application/json"):
        try:
            return json.loads(raw.decode("utf-8"))
        except (json.JSONDecodeError, UnicodeDecodeError) as e:
            raise _errors.PlatformError(
                f"platform returned {status} but JSON was malformed: {e}"
            ) from e
    # Non-JSON 2xx — return raw text. Rare for /api/v1 endpoints.
    try:
        return raw.decode("utf-8")
    except UnicodeDecodeError:
        return raw  # bytes fallback


def _raise_for_http_error(http_err: urllib.error.HTTPError) -> None:
    """Map urllib HTTPError (4xx/5xx) to our typed exception hierarchy."""
    status = http_err.code
    try:
        body_excerpt = http_err.read().decode("utf-8", errors="replace")[:500]
    except Exception:
        body_excerpt = ""

    if status == 404:
        raise _errors.NotFoundError(f"404 Not Found: {body_excerpt}")
    if status == 403:
        raise _errors.PermissionError(f"403 Forbidden: {body_excerpt}")
    if status == 400:
        raise _errors.ValidationError(f"400 Bad Request: {body_excerpt}")
    if status >= 500:
        raise _errors.PlatformError(f"{status}: {body_excerpt}")
    # Other 4xx (401, 405, 409, 422, etc.)
    raise _errors.ApiError(f"{status}: {body_excerpt}")


def _request(method: str, url: str, headers: dict[str, str], body_bytes: bytes | None) -> Any:
    """
    Perform an HTTP request. Returns the parsed body on 2xx, raises typed
    exceptions on errors.
    """
    req = urllib.request.Request(url, data=body_bytes, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=_TIMEOUT_SECONDS) as resp:
            return _parse_success_body(resp)
    except urllib.error.HTTPError as e:
        # 4xx/5xx come through here
        _raise_for_http_error(e)
    except urllib.error.URLError as e:
        # Network error or timeout. socket.timeout is TimeoutError in 3.10+,
        # which urllib wraps in URLError(reason=TimeoutError(...)).
        reason = e.reason
        if isinstance(reason, (TimeoutError, socket.timeout)):
            raise _errors.TimeoutError(f"{method} {url} timed out") from e
        raise _errors.PlatformError(f"{method} {url} network error: {reason}") from e
    except socket.timeout as e:  # safety net for unusual socket configurations
        raise _errors.TimeoutError(f"{method} {url} timed out") from e


# ─── Public methods ────────────────────────────────────────────────────────


def get(
    path: str,
    params: dict[str, Any] | None = None,
    *,
    system: bool = False,
) -> Any:
    """
    GET the platform at `path` (relative to _CALLBACK_BASE_URL).

    Args:
        path: Server path, e.g. "/api/v1/conversations/123".
        params: Optional query params.
        system: When True, the request acts as the System user instead
            of the script's invoker. Used for reads that should not be
            attributed to the human triggerer (rare; most reads are fine
            as the invoker).

    Example:
        conv = sdk.api.get("/api/v1/conversations/abc-123")
        results = sdk.api.get("/api/v1/macros", params={"team_id": 5})
    """
    if not _BASE:
        raise _errors.PlatformError("SDK not initialized: _CALLBACK_BASE_URL is empty")
    return _request("GET", _build_url(path, params), _read_headers(system), None)


def post(
    path: str,
    body: dict[str, Any] | None = None,
    *,
    system: bool = False,
) -> Any:
    """
    POST to the platform. Idempotent under retry (per execution).

    Args:
        path: Server path.
        body: JSON body.
        system: When True, the request acts as the System user. Use this
            for actions the script takes on the platform's behalf
            ("auto-tagged based on rule X") rather than on the agent's
            behalf ("Alice replied with...").

    Example:
        sdk.api.post(
            "/api/v1/conversations/abc-123/tags",
            {"tag": "vip"},
            system=True,
        )
    """
    if not _BASE:
        raise _errors.PlatformError("SDK not initialized: _CALLBACK_BASE_URL is empty")
    body_bytes = (
        json.dumps(body, default=str, ensure_ascii=False).encode("utf-8")
        if body is not None
        else None
    )
    return _request("POST", _build_url(path), _write_headers(path, body, system), body_bytes)


def patch(
    path: str,
    body: dict[str, Any] | None = None,
    *,
    system: bool = False,
) -> Any:
    """PATCH the platform. Idempotent under retry. See post() for `system`."""
    if not _BASE:
        raise _errors.PlatformError("SDK not initialized: _CALLBACK_BASE_URL is empty")
    body_bytes = (
        json.dumps(body, default=str, ensure_ascii=False).encode("utf-8")
        if body is not None
        else None
    )
    return _request("PATCH", _build_url(path), _write_headers(path, body, system), body_bytes)


def delete(path: str, *, system: bool = False) -> Any:
    """DELETE on the platform. Idempotent under retry. See post() for `system`."""
    if not _BASE:
        raise _errors.PlatformError("SDK not initialized: _CALLBACK_BASE_URL is empty")
    return _request("DELETE", _build_url(path), _write_headers(path, None, system), None)
