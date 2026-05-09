"""
POST /v1/prewarm — pre-build an L1 snapshot for a specific package set.

Called by Go when an admin creates or updates a Python tool. After
prewarming completes, the first real execution against this package set
hits the warm cache (~100ms restore) instead of paying full cold-boot cost.
"""
from __future__ import annotations

import time

from fastapi import APIRouter, Response, status

from app.auth import RequiresServiceToken
from app.models import PrewarmRequest, PrewarmResponse
from app.observability.logging import get_logger
from app.runner.prewarm import prewarm_tool_snapshot

logger = get_logger(__name__)

router = APIRouter(prefix="/v1", tags=["prewarm"])


@router.post(
    "/prewarm",
    response_model=PrewarmResponse,
    responses={503: {"model": PrewarmResponse}},
)
async def prewarm_endpoint(
    req: PrewarmRequest,
    response: Response,
    _auth: RequiresServiceToken,
) -> PrewarmResponse:
    logger.info("prewarm.request", language=req.language, packages=req.packages)

    started = time.monotonic()
    ok = await prewarm_tool_snapshot(req.language, req.packages, source="api")
    duration_ms = int((time.monotonic() - started) * 1000)

    if not ok:
        response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return PrewarmResponse(
            ok=False,
            duration_ms=duration_ms,
            message="prewarm failed; see service logs for details",
        )

    return PrewarmResponse(ok=True, duration_ms=duration_ms)
