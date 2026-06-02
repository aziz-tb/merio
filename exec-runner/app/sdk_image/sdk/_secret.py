"""
Read-only access to operator-managed platform secrets.
"""
from __future__ import annotations

from ._data import secrets as _secrets


def get(name: str) -> str | None:
    """
    Return the plaintext value for a configured secret, or None when absent.
    """
    value = _secrets.get(name)
    if isinstance(value, str):
        return value
    return None

