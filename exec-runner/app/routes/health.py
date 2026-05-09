"""
Health check endpoints.

  GET /v1/health/live   — liveness, returns 200 if process is up
  GET /v1/health/ready  — readiness, returns 200 only if scheduler is
                          ready and warm pool is at least at degraded threshold

These do NOT require service-token auth — they're meant for load balancers
and orchestrators (k8s probes, systemd watchers).
"""
from __future__ import annotations

from fastapi import APIRouter, Response, status

from app.models import HealthLiveResponse, HealthReadyResponse
from app.runner.execute import get_gate
from app.runner.scheduler import get_warm_pool_state, is_ready

router = APIRouter(prefix="/v1/health", tags=["health"])


@router.get("/live", response_model=HealthLiveResponse)
async def live() -> HealthLiveResponse:
    """Process-is-up probe. Always 200 if we can serve the request at all."""
    return HealthLiveResponse(status="alive")


@router.get(
    "/ready",
    response_model=HealthReadyResponse,
    responses={503: {"model": HealthReadyResponse}},
)
async def ready(response: Response) -> HealthReadyResponse:
    """
    Traffic-readiness probe.

    Returns:
      200 with status="ready"     — scheduler ready, warm pool at target
      200 with status="degraded"  — scheduler ready, warm pool below target but >=50%
      503 with status="starting"  — scheduler not yet ready
    """
    scheduler_running = is_ready()

    if not scheduler_running:
        response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return HealthReadyResponse(
            status="starting",
            scheduler_running=False,
            warm_pool={
                "python": {"target": 0, "current": 0},
                "javascript": {"target": 0, "current": 0},
            },
            concurrent_executions=0,
            max_concurrent=0,
        )

    pool_state = get_warm_pool_state()

    # Degraded threshold: serving traffic if pool is at least 50% of target
    # (and target > 0). This avoids flapping during background re-warming.
    py = pool_state["python"]
    js = pool_state["javascript"]

    def _below_threshold(p: dict[str, int]) -> bool:
        if p["target"] == 0:
            return False  # JS pool may legitimately be 0 if not in use
        return p["current"] < p["target"] // 2

    is_degraded_below_threshold = _below_threshold(py) or _below_threshold(js)

    is_at_target = (py["current"] >= py["target"]) and (
        js["target"] == 0 or js["current"] >= js["target"]
    )

    gate = get_gate()
    in_flight = gate.in_flight
    max_concurrent = gate.max_concurrent

    if is_degraded_below_threshold:
        # Below 50% of target — return 503, don't accept traffic yet.
        response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return HealthReadyResponse(
            status="starting",
            scheduler_running=True,
            warm_pool=pool_state,
            concurrent_executions=in_flight,
            max_concurrent=max_concurrent,
        )

    health_status = "ready" if is_at_target else "degraded"
    return HealthReadyResponse(
        status=health_status,
        scheduler_running=True,
        warm_pool=pool_state,
        concurrent_executions=in_flight,
        max_concurrent=max_concurrent,
    )
