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

# Read context lazily only if this tool needs it
conversation = sdk.context.conversation()
if conversation is None:
    sdk.error("no ticket attached")
    raise SystemExit

ticket_id = conversation["id"]

# Call the platform
try:
    contract = sdk.api.get(f"/api/v1/contracts/{contract_id}", system=True)
except sdk.NotFoundError:
    sdk.failure(f"contract {contract_id} not found", code="NO_CONTRACT")
    raise SystemExit

# Do work
if contract["delivery_state"] == "active":
    sdk.api.post(
        f"/api/v1/contacts/{conversation['contact_id']}/notes",
        {"content": f"Active contract: {contract['rate']}"},
        system=True,
    )
    sdk.success(data={"contract_id": contract_id, "rate": contract["rate"]})
else:
    sdk.failure("contract is not active", code="INACTIVE")
```

## Inputs

```python
sdk.input          # dict of LLM-provided tool arguments (always a dict, never None)
```

Use lazy context helpers when a tool needs conversation, contract, or customer
context:

```python
conversation = sdk.context.conversation()  # full Conversation dict, or None
contract = sdk.context.contract()          # linked contract dict, or None
customer = sdk.context.customer()          # linked customer dict, or None
```

The helpers return `None` outside conversation scope. `contract()` and
`customer()` also return `None` when the conversation is unverified or has no
linked record. Results are memoized within one execution, so repeated calls do
not repeat platform reads.

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

**Do not print secret values.** There is currently **no** automatic redaction of
script output — anything a script writes to stdout, stderr, or a result
message/data field is stored verbatim in the execution audit log.

> Planned (not yet implemented): automatic replacement of leaked secret values
> with `[REDACTED:<secret_name>]` before storage.

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
sdk.api.post(path, body=None)        # Writes — NOT deduplicated server-side (see note below)
sdk.api.patch(path, body=None)
sdk.api.delete(path)
```

Returns parsed JSON for 2xx responses. Raises typed exceptions for errors:

```python
try:
    customer = sdk.api.get("/api/v1/customers/999", system=True)
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

### Idempotency (NOT yet implemented — writes are not deduplicated)

POST/PATCH/DELETE calls automatically include an `Idempotency-Key` header
derived from `(execution_id, path, body)`. **The platform does not yet consume
this header** — there is no server-side replay cache, so the key is not
deduplicated.

This means: if your script crashes (or the run is retried) after a successful
write but before emitting `__RESULT__`, the retry **will** re-execute the write.
Treat writes as non-idempotent and design tools defensively (e.g. check before
write).

> Planned: a server-side ~1-hour replay cache keyed on `Idempotency-Key`. Until
> then the header is sent but ignored.

## Deprecated tool composition

```python
sdk.tools.call("search_glossary", {"term": "BNetzA"})
```

`sdk.tools.call` is deprecated and always raises `sdk.DeprecationError`.
Use `sdk.api.get/post/patch/delete` for platform calls, or
`sdk.context.conversation()/contract()/customer()` for execution context.

## External APIs

For calling external APIs (your customer's CRM, an ERP, etc.), use plain
`requests`:

```python
import requests
resp = requests.get("https://api.example.com/data", timeout=10)
```

Outbound network is enabled for sandbox scripts in this deployment. The
legacy `allowed_domains` request field is retained for compatibility but is
not enforced by the runner.

The SDK is for **platform** calls only. External calls are not the SDK's
job, and shouldn't be — this keeps the SDK narrow.
