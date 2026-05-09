"""
Configuration loaded from environment variables.

Loaded once at startup. Fails fast if required values are missing or invalid.
"""
from __future__ import annotations

import os
from dataclasses import dataclass


def _env_str(name: str, default: str | None = None, required: bool = False) -> str:
    val = os.environ.get(name, default)
    if required and (val is None or val == ""):
        raise RuntimeError(f"Required environment variable {name} is not set")
    return val or ""


def _env_int(name: str, default: int) -> int:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    try:
        return int(val)
    except ValueError as e:
        raise RuntimeError(f"Environment variable {name}={val!r} is not a valid integer") from e


def _env_bool(name: str, default: bool) -> bool:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    return val.lower() in ("1", "true", "yes", "on")


@dataclass(frozen=True)
class Config:
    # Service identity
    service_token: str
    service_port: int

    # Sandbox sizing
    warm_pool_size_python: int
    warm_pool_size_js: int
    default_memory_mb: int
    default_timeout_seconds: int

    # Concurrency
    max_concurrent_executions: int
    execution_queue_size: int

    # Behavior
    prewarm_on_startup: bool
    enable_package_validation: bool

    # Observability
    log_level: str
    log_format: str
    metrics_port: int

    @classmethod
    def from_env(cls) -> "Config":
        # Force EXEC_SANDBOX_OFFLINE=1 in production deployments;
        # we don't enforce it here because some tests need the network.
        cfg = cls(
            service_token=_env_str("SERVICE_TOKEN", required=True),
            service_port=_env_int("SERVICE_PORT", 8090),
            warm_pool_size_python=_env_int("WARM_POOL_SIZE_PYTHON", 10),
            warm_pool_size_js=_env_int("WARM_POOL_SIZE_JS", 5),
            default_memory_mb=_env_int("DEFAULT_MEMORY_MB", 256),
            default_timeout_seconds=_env_int("DEFAULT_TIMEOUT_SECONDS", 60),
            max_concurrent_executions=_env_int("MAX_CONCURRENT_EXECUTIONS", 64),
            execution_queue_size=_env_int("EXECUTION_QUEUE_SIZE", 128),
            prewarm_on_startup=_env_bool("PREWARM_ON_STARTUP", True),
            enable_package_validation=_env_bool("ENABLE_PACKAGE_VALIDATION", True),
            log_level=_env_str("LOG_LEVEL", "INFO"),
            log_format=_env_str("LOG_FORMAT", "json"),
            metrics_port=_env_int("METRICS_PORT", 9090),
        )

        # Validate ranges
        if cfg.default_memory_mb < 128:
            raise RuntimeError(f"DEFAULT_MEMORY_MB={cfg.default_memory_mb} must be >= 128")
        if cfg.default_timeout_seconds < 1 or cfg.default_timeout_seconds > 300:
            raise RuntimeError(
                f"DEFAULT_TIMEOUT_SECONDS={cfg.default_timeout_seconds} must be in [1, 300]"
            )
        if cfg.max_concurrent_executions < 1:
            raise RuntimeError(
                f"MAX_CONCURRENT_EXECUTIONS={cfg.max_concurrent_executions} must be >= 1"
            )
        if cfg.log_format not in ("json", "console"):
            raise RuntimeError(f"LOG_FORMAT={cfg.log_format!r} must be 'json' or 'console'")

        return cfg
