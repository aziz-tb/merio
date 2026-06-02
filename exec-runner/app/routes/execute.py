"""
POST /v1/execute — run a single script.

Returns 200 with the structured result envelope when the script ran
(regardless of script-side success/failure), or 503 when the sandbox itself
failed (VM crash, capacity, etc.) — the Go side decides whether to retry.
"""
from __future__ import annotations

import traceback

from fastapi import APIRouter, Response, status
from pydantic import ValidationError

from app.auth import RequiresServiceToken
from app.models import (
    ExecuteErrorResponse,
    ExecuteRequest,
    ExecuteResponse,
    ExecuteRuntimeErrorResponse,
)
from app.observability.logging import bind_trace, clear_trace, get_logger
from app.runner.execute import execute

logger = get_logger(__name__)

router = APIRouter(prefix="/v1", tags=["execute"])


@router.post(
    "/execute",
    response_model=None,  # union response — we set status_code manually
    responses={
        200: {"model": ExecuteResponse},
        500: {"model": ExecuteRuntimeErrorResponse},
        503: {"model": ExecuteErrorResponse},
    },
)
async def execute_endpoint(
    req: ExecuteRequest,
    response: Response,
    _auth: RequiresServiceToken,
) -> ExecuteResponse | ExecuteErrorResponse | ExecuteRuntimeErrorResponse:
    bind_trace(req.trace_id)
    try:
        logger.info(
            "execute.start",
            execution_id=req.execution_id,
            language=req.language,
            timeout_seconds=req.timeout_seconds,
            memory_mb=req.memory_mb,
            packages_count=len(req.packages),
            tool_depth=req.tool_depth,
        )

        result = await execute(req)

        if isinstance(result, ExecuteErrorResponse):
            response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return result
    except Exception as e:
        tb = traceback.format_exc()
        code = "INVALID_ENVELOPE" if isinstance(e, ValidationError) else "UNCAUGHT"
        message = str(e) or type(e).__name__
        logger.exception(
            "execute.runtime_error",
            execution_id=req.execution_id,
            code=code,
            error=message,
        )
        response.status_code = status.HTTP_500_INTERNAL_SERVER_ERROR
        return ExecuteRuntimeErrorResponse(
            code=code,
            message=message,
            traceback=tb,
        )
    finally:
        clear_trace()
