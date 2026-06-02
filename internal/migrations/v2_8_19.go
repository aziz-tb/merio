package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_8_19 seeds starter code and default preview parameters for the four
// customer-identification functions. It only updates rows whose code is still
// empty, so operator-edited functions are never overwritten.
func V2_8_19(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
UPDATE functions
SET code = $code$
import sdk
import requests

# Inputs come from the AI's parameter extraction (configured in
# the "AI-based identification" admin section). Any subset may be
# present; absent fields are None.
contract_id = sdk.input.get("contractId")
customer_id = sdk.input.get("customerId")
firstname = sdk.input.get("firstname")
lastname = sdk.input.get("lastname")
email = sdk.input.get("email")
phone = sdk.input.get("phone")
zip_code = sdk.input.get("zip")
city = sdk.input.get("city")
company = sdk.input.get("company")
ticket_id = sdk.input.get("ticketId")

# Reject calls with no useful inputs.
if not any([contract_id, customer_id, firstname, lastname, email, phone, company]):
    sdk.failure("no identifying attributes provided", code="MISSING_INPUT")
    raise SystemExit

# TODO: replace with your ERP endpoint + auth.
# Example shape - adjust to your API.
try:
    resp = requests.get(
        "https://your-erp.example.com/api/customers/search",
        params={
            "contractId": contract_id,
            "customerId": customer_id,
            "firstName": firstname,
            "lastName": lastname,
            "email": email,
        },
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure("no customer matched the provided attributes", code="NOT_FOUND")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

# Return AT MOST ONE match. If multiple candidates exist, return the
# most likely one. Empty {} is also a valid "no match" outcome via failure().
match = resp.json()

# Expected output shape:
#
#   {
#     "contractId": "<external contract id>",   # required
#     "customerId": "<external customer id>",   # required
#
#     # Optional override of the verification flow:
#     #   "customerLegitimation": 20,
#     #   "customerLegitimationMessage": "<reason if not 20>",
#     # When set, the engine uses these values directly and skips
#     # the rule evaluation. Use sparingly.
#
#     # Optional preview shown to operators in the sidebar:
#     #   "agentPreview": "<short text summary>",
#   }

sdk.success(data=match)
$code$,
    parameters = '[
      {"id":"11111111-1111-4111-8111-000000000001","key":"contractId","value":""},
      {"id":"11111111-1111-4111-8111-000000000002","key":"customerId","value":""},
      {"id":"11111111-1111-4111-8111-000000000003","key":"firstname","value":""},
      {"id":"11111111-1111-4111-8111-000000000004","key":"lastname","value":""},
      {"id":"11111111-1111-4111-8111-000000000005","key":"email","value":""},
      {"id":"11111111-1111-4111-8111-000000000006","key":"phone","value":""},
      {"id":"11111111-1111-4111-8111-000000000007","key":"zip","value":""},
      {"id":"11111111-1111-4111-8111-000000000008","key":"city","value":""},
      {"id":"11111111-1111-4111-8111-000000000009","key":"company","value":""},
      {"id":"11111111-1111-4111-8111-000000000010","key":"ticketId","value":""}
    ]'::jsonb
WHERE name = 'search_customer_by_fields'
  AND domain = 'customer_identification'
  AND code = '';

UPDATE functions
SET code = $code$
import sdk
import requests

contract_id = sdk.input.get("contractId")
if not contract_id:
    sdk.failure("contractId is required", code="MISSING_INPUT")
    raise SystemExit

# TODO: fetch the contract from your ERP by ID.
try:
    resp = requests.get(
        f"https://your-erp.example.com/api/contracts/{contract_id}",
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure(f"contract {contract_id} not found", code="NO_CONTRACT")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

contract = resp.json()

# Expected output shape - the full contract record:
#
#   {
#     "id": "<external contract id>",            # required (string or int)
#     "customerId": "<external customer id>",    # required (FK to the customer)
#     "business": false,                         # required (true if commercial)
#     "lastname": "<surname>",                   # required if business=false
#     "company": "<company name>",               # required if business=true
#     "firstname": "<given name>",               # optional
#     "email": "<contact email>",                # optional
#     "phone": "<contact phone>",                # optional
#     "deliveryAddress": "<full delivery address>",
#     "billingAddress": "<full billing address>",
#     "zip": "<delivery zip>",
#     "agentPreview": "<short summary text>",
#     "erpUrls": [{"label": "...", "url": "..."}],
#     # Any additional fields are preserved verbatim and available
#     # to custom criteria via dot-notation (e.g., extra_data.malo).
#   }

sdk.success(data=contract)
$code$,
    parameters = '[
      {"id":"22222222-2222-4222-8222-000000000001","key":"contractId","value":""}
    ]'::jsonb
WHERE name = 'search_contract_by_id'
  AND domain = 'customer_identification'
  AND code = '';

UPDATE functions
SET code = $code$
import sdk
import requests

customer_id = sdk.input.get("customerId")
if not customer_id:
    sdk.failure("customerId is required", code="MISSING_INPUT")
    raise SystemExit

# TODO: fetch the customer from your ERP by ID.
try:
    resp = requests.get(
        f"https://your-erp.example.com/api/customers/{customer_id}",
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if resp.status_code == 404:
    sdk.failure(f"customer {customer_id} not found", code="NO_CUSTOMER")
    raise SystemExit
if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

customer = resp.json()

# Expected output shape - the full customer record:
#
#   {
#     "id": "<external customer id>",            # required
#     "business": false,                         # required (true if commercial)
#     "lastname": "<surname>",                   # required if business=false
#     "company": "<company name>",               # required if business=true
#     "contractIds": ["<id>", "<id>"],           # required; at least one
#     "firstname": "<given name>",               # optional
#     "email": "<contact email>",                # optional
#     "phone": "<contact phone>",                # optional
#     "deliveryAddress": "<full delivery address>",
#     "billingAddress": "<full billing address>",
#   }

sdk.success(data=customer)
$code$,
    parameters = '[
      {"id":"33333333-3333-4333-8333-000000000001","key":"customerId","value":""}
    ]'::jsonb
WHERE name = 'search_customer_by_id'
  AND domain = 'customer_identification'
  AND code = '';

UPDATE functions
SET code = $code$
import sdk
import requests

q = sdk.input.get("q", "")
offset = int(sdk.input.get("offset", 0))
limit = int(sdk.input.get("limit", 10))

if not q.strip():
    sdk.failure("search query is empty", code="MISSING_INPUT")
    raise SystemExit

# TODO: free-text search across customers using q.
# Used by the agent search bar at the top of the screen.
try:
    resp = requests.get(
        "https://your-erp.example.com/api/customers/search",
        params={"q": q, "offset": offset, "limit": limit},
        timeout=10,
    )
except requests.RequestException as e:
    sdk.error(f"ERP unreachable: {e}", retryable=True)
    raise SystemExit

if not resp.ok:
    sdk.error(f"ERP returned {resp.status_code}", retryable=resp.status_code >= 500)
    raise SystemExit

# Expected output shape - array of partial customer records.
# merio will fetch full details for each via search_customer_by_id,
# so this only needs to return the IDs:
#
#   [ { "id": "<external customer id>" }, ... ]

results = resp.json()
sdk.success(data=results)
$code$,
    parameters = '[
      {"id":"44444444-4444-4444-8444-000000000001","key":"q","value":""},
      {"id":"44444444-4444-4444-8444-000000000002","key":"offset","value":"0"},
      {"id":"44444444-4444-4444-8444-000000000003","key":"limit","value":"10"}
    ]'::jsonb
WHERE name = 'search_customer_by_free_text'
  AND domain = 'customer_identification'
  AND code = '';
	`)
	return err
}
