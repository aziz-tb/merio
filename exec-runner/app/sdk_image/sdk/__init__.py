"""
Platform SDK for scripts running in the execution sandbox.

Public surface:

    Data (read-only):
        sdk.input              — dict of LLM-provided arguments
        sdk.conversation_data        — dict | None
        sdk.contract_data      — dict | None
        sdk.customer_data      — dict | None

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

    Tool composition:
        sdk.tools.call(name, args)

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
        sdk.DepthLimitError       sdk.tools.call exceeded depth cap

See README.md (alongside this module) for examples.
"""
from __future__ import annotations

# Data — module-level globals populated at import time
from ._data import contract_data, conversation_data, customer_data, input

# Errors
from ._errors import (
    ApiError,
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
from . import _secret as secret
from . import _tools as tools

__all__ = [
    # Data
    "input",
    "conversation_data",
    "contract_data",
    "customer_data",
    # Outputs
    "success",
    "failure",
    "error",
    "noop",
    # Sub-modules
    "api",
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
]
