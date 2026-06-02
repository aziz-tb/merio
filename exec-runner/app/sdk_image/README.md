# Platform SDK for sandbox scripts

Available in scripts as `import sdk`.

## Quick example

```python
import sdk

# Read inputs
contract_id = sdk.input.get("contract_id")
if contract_id is None:
    sdk.failure("contract_id is required", code="MISSING_INPUT")
    raise SystemExit

# Read context (already pre-fetched by the platform)
if sdk.conversation_data is None:
    sdk.error("no ticket attached")
    raise SystemExit

ticket_id = sdk.conversation_data["id"]

# Call the platform
try:
    contract = sdk.api.get(f"/sdk/contracts/{contract_id}")
except sdk.NotFoundError:
    sdk.failure(f"contract {contract_id} not found", code="NO_CONTRACT")
    raise SystemExit

# Do work
if contract["delivery_state"] == "active":
    sdk.api.post(
        f"/sdk/tickets/{ticket_id}/notes",
        {"content": f"Active contract: {contract['rate']}"}
    )
    sdk.success(data={"contract_id": contract_id, "rate": contract["rate"]})
else:
    sdk.failure("contract is not active", code="INACTIVE")
```

## Inputs

```python
sdk.input          # dict of LLM-provided tool arguments (always a dict, never None)
sdk.conversation_data    # dict of the current ticket, or None if not in a ticket context
sdk.contract_data  # dict of the resolved contract, or None
sdk.customer_data  # dict of the resolved customer, or None
```

These are plain dicts. Use `.get("key")` for safe access — fields can be
absent or null depending on what the platform was able to resolve.

## Secrets

```python
api_key = sdk.secret.get("rabot_erp_api_key")
if api_key is None:
    sdk.error("rabot_erp_api_key not configured", retryable=False)
    raise SystemExit
```

Secrets are configured by admins in Merio and are write-only after
creation. `sdk.secret.get(name)` returns the plaintext string for this
execution or `None` when the secret does not exist. There is no SDK write API.

If a script accidentally prints a secret value to stdout, stderr, or a result
message/data field, Merio replaces the value before storage with
`[REDACTED:<secret_name>]`.

## Outputs

Call exactly one of these per execution. The last call wins if you call
multiple. **Not calling one is a bug** — the platform reports
`NO_RESULT_EMITTED`.

```python
sdk.success(data={...})              # Operation completed; data is structured result
sdk.failure(message, code=None)       # Expected failure (eligibility, missing data, etc.)
sdk.error(message, retryable=False)   # Unexpected error (upstream 5xx, etc.)
sdk.noop()                            # I ran, nothing to report (typical for hooks)
```

`success` carries data the platform consumes. `data` may be any
JSON-serializable value: dict, list, scalar, or null. `failure` and `error`
carry human-readable messages. The distinction:

- **failure** — the script worked correctly, but the answer was no/can't
- **error** — something unexpected went wrong; setting `retryable=True`
  signals to the caller that retrying may help

## Platform API

```python
sdk.api.get(path, params=None)       # Reads — naturally idempotent
sdk.api.post(path, body=None)        # Writes — auto-idempotent under retry
sdk.api.patch(path, body=None)
sdk.api.delete(path)
```

Returns parsed JSON for 2xx responses. Raises typed exceptions for errors:

```python
try:
    customer = sdk.api.get("/sdk/customers/999")
except sdk.NotFoundError:
    # 404
    pass
except sdk.PermissionError:
    # 403 — script not authorized
    pass
except sdk.PlatformError:
    # 5xx — platform issue, often retryable
    pass
except sdk.TimeoutError:
    # Network timeout
    pass
```

### Idempotency (you don't have to think about this)

POST/PATCH/DELETE calls automatically include an `Idempotency-Key` header
derived from `(execution_id, path, body)`. If the platform sees the same
key within 1 hour, it returns the cached response instead of re-executing.

This means: if your script crashes after a successful write but before
emitting `__RESULT__`, the retry will not double-execute the write.

You don't need to opt in. There's no `idempotency_key` parameter. It just works.

## Tool composition

```python
result = sdk.tools.call("search_glossary", {"term": "BNetzA"})
```

Calls another registered tool through the platform. Depth is capped at 3
to prevent loops; exceeding the cap raises `sdk.DepthLimitError`.

## External APIs

For calling external APIs (your customer's CRM, an ERP, etc.), use plain
`requests`:

```python
import requests
resp = requests.get("https://api.example.com/data", timeout=10)
```

The script's `allowed_domains` (set at dispatch time by the platform)
controls which hosts are reachable from the VM.

The SDK is for **platform** calls only. External calls are not the SDK's
job, and shouldn't be — this keeps the SDK narrow.
