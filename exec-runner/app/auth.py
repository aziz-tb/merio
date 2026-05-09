"""
Service-token authentication for Go ↔ Python service.

The Python service does NOT verify the per-execution JWT used by the SDK
for callbacks — that's the Go binary's job. This module only validates
the shared service token sent by Go on every /v1/* request.
"""
from __future__ import annotations

import hmac
from typing import Annotated

from fastapi import Depends, Header, HTTPException, Request, status

from app.config import Config


def get_config_from_state(request: Request) -> Config:
    """Pull the Config from app.state. Used by the dependency below."""
    cfg: Config | None = getattr(request.app.state, "config", None)
    if cfg is None:
        # Should never happen — lifespan sets this before requests are served.
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="service not initialized",
        )
    return cfg


async def verify_service_token(
    request: Request,
    authorization: Annotated[str | None, Header()] = None,
) -> None:
    """
    FastAPI dependency that rejects requests without a valid Bearer token.

    Uses hmac.compare_digest for constant-time comparison to avoid timing
    attacks on the shared secret.
    """
    cfg = get_config_from_state(request)

    if not authorization:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="missing Authorization header",
        )

    parts = authorization.split(" ", 1)
    if len(parts) != 2 or parts[0].lower() != "bearer":
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="malformed Authorization header",
        )

    presented = parts[1]
    if not hmac.compare_digest(presented, cfg.service_token):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid service token",
        )


# Convenience type alias for routes that require auth.
RequiresServiceToken = Annotated[None, Depends(verify_service_token)]