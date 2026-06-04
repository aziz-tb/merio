"""Deprecated tool-composition surface kept for compatibility."""
from __future__ import annotations

from typing import Any

from . import _errors

_DEPRECATION_MESSAGE = (
    "sdk.tools.call has been deprecated. "
    "Use sdk.api.get/post/patch/delete for platform API calls, "
    "or sdk.context.conversation()/contract()/customer() for tool execution context."
)


def call(name: str, args: dict[str, Any], *, system: bool = False) -> Any:
    """
    Deprecated. Kept so existing tools fail loudly with migration guidance.
    """
    raise _errors.DeprecationError(_DEPRECATION_MESSAGE)
