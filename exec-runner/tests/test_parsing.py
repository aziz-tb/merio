"""
Tests for runner/parsing.py — the __RESULT__ sentinel parser.

These don't need a live sandbox; they're pure logic.
"""
from __future__ import annotations

import json

from app.runner.parsing import parse_result_line


def test_parses_simple_success():
    stdout = '__RESULT__ {"kind": "success", "data": {"x": 1}}\n'
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "success"
    assert parsed["data"] == {"x": 1}


def test_parses_failure_with_code():
    stdout = '__RESULT__ {"kind": "failure", "message": "no contract", "code": "NO_CONTRACT"}'
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "failure"
    assert parsed["message"] == "no contract"
    assert parsed["code"] == "NO_CONTRACT"


def test_parses_error_with_retryable():
    stdout = '__RESULT__ {"kind": "error", "message": "timeout", "retryable": true}'
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "error"
    assert parsed["retryable"] is True


def test_parses_noop():
    stdout = '__RESULT__ {"kind": "noop"}'
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "noop"


def test_returns_last_sentinel_when_multiple():
    stdout = (
        '__RESULT__ {"kind": "failure", "message": "first"}\n'
        'some script log line\n'
        '__RESULT__ {"kind": "success", "data": {"final": true}}\n'
    )
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "success"
    assert parsed["data"] == {"final": True}


def test_ignores_non_sentinel_lines():
    stdout = (
        "DEBUG: starting work\n"
        "Processing item 1\n"
        '__RESULT__ {"kind": "success", "data": {}}\n'
        "extra trailing log\n"  # log line after sentinel — should still find sentinel
    )
    # Note: our parser walks in reverse and finds the LAST sentinel,
    # so a trailing log line doesn't matter.
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["kind"] == "success"


def test_returns_none_for_empty_stdout():
    assert parse_result_line("") is None


def test_returns_none_when_no_sentinel():
    stdout = "Just some log output\nNothing structured here\n"
    assert parse_result_line(stdout) is None


def test_returns_none_for_malformed_json():
    stdout = "__RESULT__ {not valid json}"
    assert parse_result_line(stdout) is None


def test_returns_none_for_non_object_json():
    # Sentinel must contain a JSON object, not an array or scalar
    assert parse_result_line('__RESULT__ ["array"]') is None
    assert parse_result_line('__RESULT__ "string"') is None
    assert parse_result_line("__RESULT__ 42") is None


def test_returns_none_for_invalid_kind():
    stdout = '__RESULT__ {"kind": "weird_kind", "data": {}}'
    assert parse_result_line(stdout) is None


def test_handles_unicode_in_payload():
    payload = {"kind": "success", "data": {"text": "Zählerstand: 42 kWh — überprüfen"}}
    stdout = f"__RESULT__ {json.dumps(payload, ensure_ascii=False)}"
    parsed = parse_result_line(stdout)
    assert parsed is not None
    assert parsed["data"]["text"] == "Zählerstand: 42 kWh — überprüfen"


def test_handles_sentinel_only_no_payload():
    # __RESULT__ with no payload after it — invalid
    assert parse_result_line("__RESULT__") is None
    assert parse_result_line("__RESULT__ ") is None
