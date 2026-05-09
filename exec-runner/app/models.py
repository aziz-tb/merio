"""
Pydantic models for the HTTP API.

These define the wire format between Go and the Python service.
Validation happens automatically via FastAPI.
"""
from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field, field_validator


class ExecuteRequest(BaseModel):
    """POST /v1/execute request body."""

    code: str = Field(..., min_length=1, max_length=1_048_576)  # 1MB cap matches sandbox
    language: Literal["python", "javascript"] = "python"

    # Data passed to scripts via env vars
    input: dict[str, Any] = Field(default_factory=dict)
    ticket_data: dict[str, Any] | None = None
    contract_data: dict[str, Any] | None = None
    customer_data: dict[str, Any] | None = None

    # SDK callback configuration
    callback_token: str = Field(..., min_length=1)
    callback_base_url: str = Field(..., min_length=1)
    execution_id: str = Field(..., min_length=1, max_length=128)
    tool_depth: int = Field(default=0, ge=0, le=10)

    # Execution limits
    timeout_seconds: int = Field(default=60, ge=1, le=300)
    memory_mb: int = Field(default=256, ge=128)
    packages: list[str] = Field(default_factory=list, max_length=50)
    allowed_domains: list[str] = Field(default_factory=list, max_length=50)

    # Tracing
    trace_id: str = Field(..., min_length=1)

    @field_validator("callback_base_url")
    @classmethod
    def validate_url(cls, v: str) -> str:
        if not (v.startswith("http://") or v.startswith("https://")):
            raise ValueError("callback_base_url must start with http:// or https://")
        return v.rstrip("/")


class ExecuteResultEnvelope(BaseModel):
    """The script's structured result, parsed from __RESULT__ sentinel."""

    kind: Literal["success", "failure", "error", "noop"]
    data: dict[str, Any] = Field(default_factory=dict)
    message: str | None = None
    code: str | None = None
    retryable: bool = False


class ExecuteResponse(BaseModel):
    """POST /v1/execute response body — successful execution path."""

    exit_code: int
    stdout: str
    stderr: str
    result: ExecuteResultEnvelope | None
    duration_ms: int
    warm_pool_hit: bool


class ExecuteErrorResponse(BaseModel):
    """POST /v1/execute response body — sandbox-level (HTTP 503) failures."""

    error: Literal[
        "vm_timeout",
        "vm_transient",
        "vm_capacity",
        "vm_permanent",
        "input_validation",
        "package_not_allowed",
        "sandbox_error",
        "asset_error",
    ]
    message: str
    retryable: bool


class PrewarmRequest(BaseModel):
    """POST /v1/prewarm request body."""

    language: Literal["python", "javascript"]
    packages: list[str] = Field(default_factory=list, max_length=50)


class PrewarmResponse(BaseModel):
    """POST /v1/prewarm response body."""

    ok: bool
    duration_ms: int | None = None
    message: str | None = None


class HealthLiveResponse(BaseModel):
    """GET /v1/health/live response."""

    status: Literal["alive"] = "alive"


class HealthReadyResponse(BaseModel):
    """GET /v1/health/ready response."""

    status: Literal["ready", "starting", "degraded"]
    scheduler_running: bool
    warm_pool: dict[str, dict[str, int]]
    concurrent_executions: int
    max_concurrent: int
