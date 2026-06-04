"""
Platform SDK for scripts running in the execution sandbox.

Public surface:

    Data (read-only):
        sdk.input              — dict of LLM-provided arguments

    Lazy context:
        sdk.context.conversation()
        sdk.context.contract()
        sdk.context.customer()

    Outputs (call exactly one to report your result):
        sdk.success(data=None)
        sdk.failure(message, code=None)
        sdk.error(message, retryable=False)
        sdk.noop()

    Platform calls (auth + idempotency are automatic):
        sdk.api.get(path, params=None)
        sdk.api.post(path, body=None)
        sdk.api.patch(path, body=None)
        sdk.api.delete(path)

    Deprecated compatibility:
        sdk.tools.call(name, args)   — raises DeprecationError

    Secrets:
        sdk.secret.get(name)

    Errors (catch with `except sdk.NotFoundError:` etc.):
        sdk.SdkError              base class
        sdk.ApiError              base for all HTTP failures
        sdk.NotFoundError         404
        sdk.PermissionError       403
        sdk.ValidationError       400
        sdk.PlatformError         5xx
        sdk.TimeoutError          network timeout
        sdk.DepthLimitError       legacy tool depth-limit error
        sdk.DeprecationError      deprecated SDK surface

See README.md (alongside this module) for examples.
"""
from __future__ import annotations

# Data — module-level globals populated at import time
from ._data import input

# Errors
from ._errors import (
    ApiError,
    DeprecationError,
    DepthLimitError,
    NotFoundError,
    PermissionError,
    PlatformError,
    SdkError,
    TimeoutError,
    ValidationError,
)

# Outputs
from ._output import error, failure, noop, success

# Sub-modules accessed via attribute (sdk.api.get, sdk.tools.call)
from . import _api as api
from . import _context as context
from . import _secret as secret
from . import _tools as tools

__all__ = [
    # Data
    "input",
    # Outputs
    "success",
    "failure",
    "error",
    "noop",
    # Sub-modules
    "api",
    "context",
    "secret",
    "tools",
    # Errors
    "SdkError",
    "ApiError",
    "NotFoundError",
    "PermissionError",
    "ValidationError",
    "PlatformError",
    "TimeoutError",
    "DepthLimitError",
    "DeprecationError",
]
