"""
Tests for the Config loader. Verifies fail-fast on bad inputs.
"""
from __future__ import annotations

import pytest

from app.config import Config


def test_loads_with_required_token(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    cfg = Config.from_env()
    assert cfg.service_token == "secret"
    assert cfg.service_port == 8090
    assert cfg.warm_pool_size_python == 10
    assert cfg.default_memory_mb == 256
    assert cfg.default_timeout_seconds == 60


def test_fails_without_token(monkeypatch):
    monkeypatch.delenv("SERVICE_TOKEN", raising=False)
    with pytest.raises(RuntimeError, match="SERVICE_TOKEN"):
        Config.from_env()


def test_overrides(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("WARM_POOL_SIZE_PYTHON", "20")
    monkeypatch.setenv("WARM_POOL_SIZE_JS", "8")
    monkeypatch.setenv("DEFAULT_MEMORY_MB", "512")
    monkeypatch.setenv("DEFAULT_TIMEOUT_SECONDS", "120")
    monkeypatch.setenv("MAX_CONCURRENT_EXECUTIONS", "100")
    monkeypatch.setenv("LOG_LEVEL", "DEBUG")
    cfg = Config.from_env()
    assert cfg.warm_pool_size_python == 20
    assert cfg.warm_pool_size_js == 8
    assert cfg.default_memory_mb == 512
    assert cfg.default_timeout_seconds == 120
    assert cfg.max_concurrent_executions == 100
    assert cfg.log_level == "DEBUG"


def test_rejects_invalid_memory(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("DEFAULT_MEMORY_MB", "64")
    with pytest.raises(RuntimeError, match="DEFAULT_MEMORY_MB"):
        Config.from_env()


def test_rejects_invalid_timeout(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("DEFAULT_TIMEOUT_SECONDS", "500")
    with pytest.raises(RuntimeError, match="DEFAULT_TIMEOUT_SECONDS"):
        Config.from_env()


def test_rejects_invalid_log_format(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("LOG_FORMAT", "xml")
    with pytest.raises(RuntimeError, match="LOG_FORMAT"):
        Config.from_env()


def test_rejects_non_integer(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("DEFAULT_MEMORY_MB", "not-a-number")
    with pytest.raises(RuntimeError, match="not a valid integer"):
        Config.from_env()


def test_bool_parsing(monkeypatch):
    monkeypatch.setenv("SERVICE_TOKEN", "secret")
    monkeypatch.setenv("PREWARM_ON_STARTUP", "false")
    cfg = Config.from_env()
    assert cfg.prewarm_on_startup is False

    monkeypatch.setenv("PREWARM_ON_STARTUP", "1")
    cfg = Config.from_env()
    assert cfg.prewarm_on_startup is True
