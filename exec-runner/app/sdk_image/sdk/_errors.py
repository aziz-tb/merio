"""
Exception classes raised by the SDK.

Scripts catch these explicitly to handle expected failure modes:

    try:
        ticket = sdk.api.get("/sdk/tickets/999")
    except sdk.NotFoundError:
        sdk.failure("ticket not found", code="NO_TICKET")
        return
    except sdk.PlatformError:
        sdk.error("platform unavailable", retryable=True)
        return

Uncaught SdkError propagates to the script's interpreter; the script exits
non-zero and the platform records the traceback.
"""
from __future__ import annotations


class SdkError(Exception):
    """Base class for all SDK-raised errors."""


class ApiError(SdkError):
    """Base class for all platform-call failures (HTTP errors)."""


class NotFoundError(ApiError):
    """Platform returned 404. Resource does not exist."""


class PermissionError(ApiError):  # noqa: A001 — shadows builtin intentionally; namespaced under sdk
    """Platform returned 403. Script not authorized for this operation."""


class ValidationError(ApiError):
    """Platform returned 400. Request shape was invalid."""


class PlatformError(ApiError):
    """Platform returned 5xx. Often retryable."""


class TimeoutError(ApiError):  # noqa: A001 — shadows builtin intentionally; namespaced under sdk
    """Network timeout reaching the platform."""


class DepthLimitError(SdkError):
    """sdk.tools.call exceeded the maximum recursion depth."""
